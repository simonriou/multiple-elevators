package main

import (
	"Driver-go/elevio"
	"Network-go/network/peers"
	"context"
	"fmt"
	"math"
	//"time"
)

func handleFloorLights(consumer3drv_floors chan int) {
	for {
		a := <-consumer3drv_floors
		elevio.SetFloorIndicator(a)
	}
}	

// obstructionToReAssignment

func handleObstruction(drv_obstr chan bool) {
	for {
		a := <-drv_obstr
		if a { // If it is on
			lockMutexes(&mutex_doors)
			ableToCloseDoors = false
			unlockMutexes(&mutex_doors)
			fmt.Print("Obstruction on\n")
		} else { // If it is off
			lockMutexes(&mutex_doors)
			ableToCloseDoors = true
			unlockMutexes(&mutex_doors)
			fmt.Print("Obstruction off\n")
		}
	}
}

/* func reAssignHallOrdersObstruction() {
	timestamp := time.Now()



	//redistributeOrders(localRequest []Order, hallBtnTx chan elevio.ButtonEvent)

} */

func handleElevatorUpdate(activeElevatorsChannelRx chan []int) {
	for {
		a := <-activeElevatorsChannelRx
		lockMutexes(&mutex_activeElevators)
		activeElevators = a
		unlockMutexes(&mutex_activeElevators)

		fmt.Printf("Active elevators: %v\n", activeElevators)
	}
}

func handleButtonPress(drv_buttons chan elevio.ButtonEvent, hallBtnTx chan elevio.ButtonEvent, d *elevio.MotorDirection, singleStateTx chan StateMsg,
	id int, drv_newOrder chan Order) {
	for {
		a := <-drv_buttons // BUTTON UPDATE

		// fmt.Print("\ndrv_buttons received button press\n")

		// If it's a hall order, forwards it to the master
		switch {
		case a.Button == elevio.BT_HallUp || a.Button == elevio.BT_HallDown: // If it's a hall order

			//fmt.Printf("Hall order from drv_buttons waiting to be sent to the master...\n")
			hallBtnTx <- a // Send the hall order to the master
			//fmt.Printf("Hall order from drv_buttons sent to the master!\n\n")

		case a.Button == elevio.BT_Cab: // Else (it's a cab)

			lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
			addOrder(a.Floor, 0, cab)                    // Add the cab order to the local elevatorOrders
			sortAllOrders(&elevatorOrders, *d, posArray) // Sort the orders
			first_element := elevatorOrders[0]

			// Update & send the new state of the elevator to the master
			updateState(d, lastFloor, elevatorOrders, &latestState)
			singleStateTx <- StateMsg{id, latestState}
			fmt.Print("Sent from handleButtonPress\n")
			unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

			//fmt.Printf("Cab order from drv_buttons waiting to be sent to attend to specificOrders...\n")
			drv_newOrder <- first_element // Send the first element of the elevatorOrders to the driver
			//fmt.Printf("Cab order from drv_buttons sent to attend to specificOrders!\n\n")
		}
	}
}

func handleNewFloorReached(consumer1drv_floors chan int, d *elevio.MotorDirection, singleStateTx chan StateMsg, id int) {
	for {
		a := <-consumer1drv_floors
		lastFloor = a // Update the last floor

		// Update & send the new state of the elevator to the master

		updateState(d, lastFloor, elevatorOrders, &latestState)
		singleStateTx <- StateMsg{id, latestState}
		fmt.Print("Sent from handleNewFloorReached\n")
	}
}

func handleStopButton(drv_stop chan bool, d *elevio.MotorDirection, id int, activeElevatorsChannelTx chan []int, hallBtnTx chan elevio.ButtonEvent) {
	for {
		a := <-drv_stop // STOP BUTTON
		switch {
		case a:
			// Rising edge, from unpressed to pressed
			lockMutexes(&mutex_d)

			// Stop the elevator
			elevio.SetStopLamp(true)
			lastDirForStopFunction = *d // Save the last direction before stopping, ## PLACEHOLDER ##
			elevio.SetMotorDirection(elevio.MD_Stop)

			unlockMutexes(&mutex_d)

			// The elevator removes himself from the activeElevators list and sends it to the other elevators
			mutex_activeElevators.Lock()
			alreadyExists := isElevatorActive(id)
			mutex_activeElevators.Unlock()
			if alreadyExists {
				mutex_activeElevators.Lock()
				removeElevator(id)
				mutex_activeElevators.Unlock()

				activeElevatorsChannelTx <- activeElevators
			}

			// Re-assign the hall orders, i.e. send them again to the master
			for _, order := range elevatorOrders {
				if order.OrderType == hall {
					hallBtnTx <- elevio.ButtonEvent{Button: elevio.ButtonType(order.Direction), Floor: order.Floor}
				}
			}

		case !a:
			// Falling edge, from pressed to unpressed
			lockMutexes(&mutex_d)
			elevio.SetMotorDirection(lastDirForStopFunction) // Start the elevator again in the last direction ## PLACEHOLDER ##
			unlockMutexes(&mutex_d)

			elevio.SetStopLamp(false)

			// The elevator adds himself to the activeElevators list and sends it to the other elevators
			mutex_activeElevators.Lock()
			alreadyExists := isElevatorActive(id)
			mutex_activeElevators.Unlock()
			if !alreadyExists {
				mutex_activeElevators.Lock()
				activeElevators = append(activeElevators, id)
				activeElevators = sortElevators(activeElevators)
				mutex_activeElevators.Unlock()

				activeElevatorsChannelTx <- activeElevators
			}
		}
	}
}

func handleNewHallOrder(hallOrderRx chan HallOrderMsg, id int, d *elevio.MotorDirection, singleStateTx chan StateMsg, drv_newOrder chan Order, hallOrderCompletedTx chan []Order) {
	for {
		a := <-hallOrderRx // NEW ORDER FROM THE MASTER
		// We turn up the lights on all slaves' servers
		turnOnHallLights(a.HallOrder)

		currentPos := extractPos()
		if float64(currentPos) == math.Trunc(float64(currentPos)) { // We are at a floor
			if math.Trunc(float64(currentPos)) == float64(a.HallOrder.Floor) && isWaiting {
				// Skip this loop iteration if the elevator is at the floor and is waiting

				hallOrderCompletedTx <- []Order{a.HallOrder}

				continue
			}
		}

		// Checking if we are the elevator that should take the order
		if a.Id == id {

			newHallOrder := a.HallOrder

			// fmt.Print("Attempting to lock mutexes\n")
			lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
			// fmt.Print("Mutexes locked\n")

			addOrder(newHallOrder.Floor, newHallOrder.Direction, hall) // Add the hall order to the local elevatorOrders
			sortAllOrders(&elevatorOrders, *d, posArray)               // Sort the orders
			first_element := elevatorOrders[0]

			// Update & send the new state of the elevator to the master
			updateState(d, lastFloor, elevatorOrders, &latestState)
			singleStateTx <- StateMsg{id, latestState}
			fmt.Print("Sent from handleNewHallOrder\n")

			// fmt.Print("Attempting to unlock mutexes\n")
			unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
			// fmt.Print("Mutexes unlocked\n")

			drv_newOrder <- first_element // Send the first element of the elevatorOrders to the driver
		}
	}
}

func handlePeerUpdate(peerUpdateCh chan peers.PeerUpdate, currentRole string, activeElevatorsChannelTx chan []int, backupStatesRx chan [numElev]ElevState,
	hallBtnRx chan elevio.ButtonEvent, singleStateRx chan StateMsg, hallOrderTx chan HallOrderMsg,
	backupStatesTx chan [numElev]ElevState, newStatesRx chan [numElev]ElevState, hallOrderCompletedTx chan []Order, retrieveCabOrdersTx chan CabOrderMsg, askForCabOrdersRx chan int,
	newStatesTx chan [numElev]ElevState, roleChannel chan string, hallBtnTx chan elevio.ButtonEvent, id int, ctx context.Context, cancel context.CancelFunc,
	allStatesFromMasterTx chan [numElev]ElevState, singleStateFromSlaveRx chan StateMsg) {
	for {
		p := <-peerUpdateCh // PEER UPDATE
		var mPeers = p.Peers
		var mNew = p.New
		var mLost = p.Lost

		// Display the peer update
		fmt.Printf("Peer update:\n")
		fmt.Printf("  Peers:    %v\n", mPeers)
		fmt.Printf("  New:      %v\n", mNew)
		fmt.Printf("  Lost:     %v\n", mLost)

		switch { // Lost or New Peer?
		case mNew != (peers.ElevIdentity{}): // A new peer joins the network

			// We want to force the new peer to be a regular elevator, BUT only if it's an elevator that was down.
			// The issue is, when an elevator looses network, it thinks that the two other ones are down.
			// Thus it becomes automatically a master.
			// We need to force it to become a regular elevator when it joins back the network.

			// Step 1: Make sure that this is indeed a recovered elevator, not a new startup

			// The master updates the activeElevators array and sends it to the other elevators
			if currentRole == "Master" {
				mutex_activeElevators.Lock()
				alreadyExists := isElevatorActive(mNew.Id) // Check if the elevator is already active

				if !alreadyExists {
					activeElevators = append(activeElevators, mNew.Id) // Add the elevator to the activeElevators list
				}

				activeElevators = sortElevators(activeElevators) // Sort for the mapping to remain correct (see communication.go)
				mutex_activeElevators.Unlock()

				activeElevatorsChannelTx <- activeElevators // Send the activeElevators list to the other elevator
			}

		case len(mLost) > 0: // A peer leaves the network

			lostElevator := mLost[0] // We assume that we only have one down elevator at a time
			fmt.Printf("We lost elevator ID %v with role %s\n", lostElevator.Id, lostElevator.Role)

			// Section_START -- CHANGING ROLES
			newRole := currentRole
			_ = newRole

			switch lostElevator.Role {
			case "Master": // The master leaves the network
				// The Regular becomes PrimaryBackup & the PrimaryBackup becomes Master
				switch currentRole {
				case "Regular":

					// Switch role to primary backup and launch it
					newRole = "PrimaryBackup"
					go PrimaryBackupRoutine(backupStatesRx)

				case "PrimaryBackup":

					newRole = "Master"
					go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx, backupStatesTx, newStatesRx, hallOrderCompletedTx,
						retrieveCabOrdersTx, askForCabOrdersRx, ctx, hallBtnTx, activeElevatorsChannelTx, allStatesFromMasterTx,
						singleStateFromSlaveRx)
					newStatesTx <- backupStates // Sending the backupStates to the new master

				}
			case "PrimaryBackup": // The PrimaryBackup leaves the network
				// The Regular becomes PrimaryBackup
				if currentRole == "Regular" {
					newRole = "PrimaryBackup"
					go PrimaryBackupRoutine(backupStatesRx)
				}
			}

			fmt.Print("Length of mPeers: ", len(mPeers), "\n")
			fmt.Print("Length of mLost: ", len(mLost), "\n")

			if len(mPeers) == 0 && len(mLost) > 0 { // This means that we were disconnected from the network
				newRole = "Regular"
				fmt.Print("Cancelling...\n")
				cancel()
				fmt.Print("Cancelled\n")
			}

			if newRole != currentRole {
				currentRole = newRole
				roleChannel <- currentRole
			}

			fmt.Printf("My new current role: %s\n", currentRole) // ## PLACEHOLDER ##

			// The new master updates the activeElevator list and sends it to the other elevators
			if currentRole == "Master" {
				mutex_activeElevators.Lock()
				alreadyExists := isElevatorActive(lostElevator.Id)

				if alreadyExists {
					removeElevator(lostElevator.Id) // Remove the elevator from the activeElevators list
				}

				activeElevators = sortElevators(activeElevators)
				mutex_activeElevators.Unlock()

				activeElevatorsChannelTx <- activeElevators // Send the activeElevators list to the other elevators
			}

			// Section_END -- CHANGING ROLES

			// Section_START -- RE-ASSIGNING ORDERS
			// Re-assign the orders of the lost elevator. This is the job of the master
			if currentRole == "Master" { // This works because we are sure that there are a Master & a Backup at all times
				// Get the lost orders
				lostOrders := backupStates[lostElevator.Id].LocalRequests

				// Re-assign the orders
				for _, order := range lostOrders {
					if order.OrderType == hall {
						// We can just send the hall order to the master
						// because it only takes into account the elevators that are inside of the activeElevators list
						// and the lost elevator is not in it
						hallBtnTx <- elevio.ButtonEvent{Button: elevio.ButtonType(order.Direction), Floor: order.Floor}
					}
				}
			}
			// Section_END -- RE-ASSIGNING ORDERS
		}

		// Display role changes
		fmt.Printf("I am elevator ID %v with role %s\n", id, currentRole)
	}
}

func handleTurnOffLightsHallOrderCompleted(hallOrderCompletedLightsRx chan []Order) {
	for {
		a := <-hallOrderCompletedLightsRx // HALL ORDER COMPLETED
		turnOffHallLights(a...)
	}
}

func handleTurnOffLightsCabOrderCompleted(localStatesForCabOrders chan StateMsg) {
	for {
		a := <-localStatesForCabOrders
		var newCabOrders []Order

		// Only keep the cab orders
		for _, order := range a.State.LocalRequests {
			if order.OrderType == cab {
				newCabOrders = append(newCabOrders, order)
			}
		}

		// Turn off all the cab lights
		for f := 0; f < numFloors; f++ {
			elevio.SetButtonLamp(elevio.BT_Cab, f, false)
		}

		// Turn on all the remaining cab orders
		for _, order := range newCabOrders {
			elevio.SetButtonLamp(elevio.BT_Cab, order.Floor, true)
		}
	}
}

func handleTurnOnLightsCabOrder(drv_buttons_forCabLights chan elevio.ButtonEvent) {
	for {
		a := <-drv_buttons_forCabLights
		if a.Button == elevio.BT_Cab {
			turnOnCabLights(Order{a.Floor, 0, cab})
		}

	}
}

func handleRetrieveCab(retrieveCabOrdersRx chan CabOrderMsg, id int, d *elevio.MotorDirection, singleStateTx chan StateMsg,
	drv_newOrder chan Order) {
	for {
		p := <-retrieveCabOrdersRx // RETRIEVE CAB ORDERS
		if p.Id == id {
			for _, order := range p.CabOrders {

				turnOnCabLights(Order{order.Floor, 0, cab})
				// Lock to safely add order and sort

				lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

				addOrder(order.Floor, order.Direction, cab)  // Add the hall order to the local elevatorOrders
				sortAllOrders(&elevatorOrders, *d, posArray) // Sort the orders

				// Copy the first element locally to avoid holding the mutex longer
				first_element := elevatorOrders[0]

				// Update & send the new state of the elevator to the master
				updateState(d, lastFloor, elevatorOrders, &latestState)
				singleStateTx <- StateMsg{id, latestState}
				fmt.Print("Sent from handleRetrieveCab\n")

				unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

				// Send to driver outside of mutex lock to prevent blocking
				drv_newOrder <- first_element // Send the first element of the elevatorOrders to the driver

			}
		}
	}
}

func receiveSpamFromMaster(allStatesFromMasterRx chan [numElev]ElevState, id int) {
	for {
		a := <-allStatesFromMasterRx // ALL STATES FROM MASTER
		myState := a[id]

		mutex_elevatorOrders.Lock()
		elevatorOrders = myState.LocalRequests // Update the local orders array
		mutex_elevatorOrders.Unlock()

		// fmt.Printf("What I received from master: %v\n", a)
	}
}

func receiveSpamFromSlave(singleStateFromSlaveRx chan StateMsg) {
	for {
		a := <-singleStateFromSlaveRx
		slaveID := a.Id
		slaveState := a.State

		mutex_backup.Lock()
		backupStates[slaveID] = slaveState // Update the backup states array
		mutex_backup.Unlock()

		// fmt.Printf("Received state from slave %d: %v\n", slaveID, slaveState)
	}
}
