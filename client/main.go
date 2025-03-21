package main

import (
	"Driver-go/elevio"
	"Network-go/network/bcast"
	"Network-go/network/peers"
	"fmt"
	"time"
)

const numFloors = 4 // Number of floors
const numElev = 3   // Number of elevators

func main() {
	// Section_START -- FLAGS & ROLE
	port, initialRole, id := getFlags()
	roleChannel := make(chan string)
	// Section_END -- FLAGS

	currentRole := initialRole

	// Section_START -- CHANNELS
	peerUpdateCh := make(chan peers.PeerUpdate)                           // Updates from peers
	peerTxEnable := make(chan bool)                                       // Enables/disables the transmitter
	go peers.Transmitter(PeerChannel_PORT, id, roleChannel, peerTxEnable) // Broadcast role
	roleChannel <- currentRole
	go peers.Receiver(PeerChannel_PORT, peerUpdateCh) // Listen for updates

	// Initialize the elevator
	elevio.Init("localhost:"+port, numFloors)

	// Channels for the driver
	drv_buttons := make(chan elevio.ButtonEvent, 100)
	drv_floors := make(chan int)
	drv_floors2 := make(chan int)
	drv_obstr := make(chan bool)
	drv_stop := make(chan bool)
	drv_newOrder := make(chan Order)
	drv_DirectionChange := make(chan elevio.MotorDirection)

	go elevio.PollButtons(drv_buttons)         // Button updates
	go elevio.PollFloorSensor(drv_floors)      // Floors updates
	go elevio.PollFloorSensor2(drv_floors2)    // Floors updates (for tracking position)
	go elevio.PollObstructionSwitch(drv_obstr) // Obstruction updates
	go elevio.PollStopButton(drv_stop)         // Stop button presses

	// Channels for the network
	hallBtnTx := make(chan elevio.ButtonEvent)   // ALL - Send hall orders to the master
	hallOrderRx := make(chan HallOrderMsg)       // ALL - Receive hall orders from the master
	singleStateTx := make(chan StateMsg)         // ALL - Send the state of the elevator to the master
	hallOrderCompletedRx := make(chan []Order)   // ALL - Confirm hall order (for lights)
	activeElevatorsChannelTx := make(chan []int) // ALL - The channel on which we send the active elevators list
	activeElevatorsChannelRx := make(chan []int) // ALL - The channel on which we receive the active elevators list

	go bcast.Receiver(HallOrder_PORT, hallOrderRx)
	go bcast.Transmitter(HallOrderRawBTN_PORT, hallBtnTx)
	go bcast.Transmitter(SingleElevatorState_PORT, singleStateTx)
	go bcast.Receiver(HallOrderCompleted_PORT, hallOrderCompletedRx)
	go bcast.Receiver(ActiveElevators_PORT, activeElevatorsChannelRx)
	go bcast.Transmitter(ActiveElevators_PORT, activeElevatorsChannelTx)

	// Channels for specific roles
	hallBtnRx := make(chan elevio.ButtonEvent)      // MASTER - Receive hall orders from slaves
	hallOrderTx := make(chan HallOrderMsg)          // MASTER - Send hall orders to slaves
	singleStateRx := make(chan StateMsg)            // MASTER - Receive states from slaves
	backupStatesRx := make(chan [numElev]ElevState) // BACKUP - Receive all states from master
	backupStatesTx := make(chan [numElev]ElevState) // MASTER - Send all states to backup
	newStatesRx := make(chan [numElev]ElevState)    // MASTER - Receive all NEW states from backup
	newStatesTx := make(chan [numElev]ElevState)    // BACKUP - Send all states to the NEW master
	hallOrderCompletedTx := make(chan []Order)

	go bcast.Transmitter(BackupStates_PORT, newStatesTx) // LOCAL - Used to send the states to the NEW master (used in role changes)

	_ = hallBtnRx
	_ = hallOrderTx
	_ = singleStateRx
	_ = backupStatesTx
	_ = newStatesRx

	// Section_END -- CHANNELS

	// Section_START -- ROLES-SPECIFIC ACTIONS
	switch role {
	case "Master":

		activeElevators = append(activeElevators, id) // Add the master to the activeElevators list

		// Starting the Master Routine
		go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx, backupStatesTx, newStatesRx, hallOrderCompletedTx)

		// This is the initial states of the elevators
		var allStates [numElev]ElevState
		uninitializedOrderArray := []Order{
			{
				Floor:     0,
				Direction: up,
				OrderType: hall,
			},
		}
		uninitialized_ElevState := ElevState{
			Behavior:      "Uninitialized",
			Floor:         -2,
			Direction:     "Uninitialized",
			LocalRequests: uninitializedOrderArray,
		}

		for i := range allStates {
			allStates[i] = uninitialized_ElevState
		}

		// Send the initial states to the master
		newStatesRx <- allStates

		fmt.Print("a\n")

	case "PrimaryBackup":

		// Starting the PrimaryBackup Routine
		go PrimaryBackupRoutine(backupStatesRx)

	}
	// Section_END -- ROLES-SPECIFIC ACTIONS

	// Section_START -- LOCAL INITIALIZATION
	var d elevio.MotorDirection = elevio.MD_Down // Setting the initial direction of the elevator

	// Initialize the elevator - going to ground floor
	initSingleElev(d, drv_floors)

	consumer1drv_floors := make(chan int) // Consumers for the drv_floors (relay)
	consumer2drv_floors := make(chan int)
	go relay(drv_floors, consumer1drv_floors, consumer2drv_floors)

	d = elevio.MD_Stop // Update d so that states are accurate
	updateState(&d, 0, elevatorOrders, &latestState)

	// Send the initial state of the elevator to the master
	singleStateTx <- StateMsg{id, latestState}

	// Starting the goroutines for tracking the position of the elevator & attending to specific orders
	go trackPosition(drv_floors2, drv_DirectionChange, &d) // Starts tracking the position of the elevator
	go attendToSpecificOrder(&d, consumer2drv_floors, drv_newOrder, drv_DirectionChange, singleStateTx, id)
	// Section_END -- LOCAL INITIALIZATION

	for { // MAIN LOOP
		select {

		case a := <-activeElevatorsChannelRx: // ACTIVE ELEVATORS UPDATE
			lockMutexes(&mutex_activeElevators)
			activeElevators = a
			unlockMutexes(&mutex_activeElevators)

			fmt.Printf("Active elevators: %v\n", activeElevators)

		case a := <-drv_buttons: // BUTTON UPDATE
			time.Sleep(30 * time.Millisecond) // Poll rate of the buttons

			// If it's a hall order, forwards it to the master
			lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

			switch {
			case a.Button == elevio.BT_HallUp || a.Button == elevio.BT_HallDown: // If it's a hall order

				hallBtnTx <- a // Send the hall order to the master

			case a.Button == elevio.BT_Cab: // Else (it's a cab)
				turnOnCabLights(Order{a.Floor, 0, cab})

				addOrder(a.Floor, 0, cab)                   // Add the cab order to the local elevatorOrders
				sortAllOrders(&elevatorOrders, d, posArray) // Sort the orders
				first_element := elevatorOrders[0]

				// Update & send the new state of the elevator to the master
				updateState(&d, lastFloor, elevatorOrders, &latestState)
				singleStateTx <- StateMsg{id, latestState}

				drv_newOrder <- first_element // Send the first element of the elevatorOrders to the driver
			}

			unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

		case a := <-hallOrderRx: // NEW ORDER FROM THE MASTER
			// We turn up the lights on all slaves' servers
			turnOnHallLights(a.HallOrder)

			// Checking if we are the elevator that should take the order
			if a.Id == id {

				newHallOrder := a.HallOrder

				lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

				addOrder(newHallOrder.Floor, newHallOrder.Direction, hall) // Add the hall order to the local elevatorOrders
				sortAllOrders(&elevatorOrders, d, posArray)                // Sort the orders
				first_element := elevatorOrders[0]

				// Update & send the new state of the elevator to the master
				updateState(&d, lastFloor, elevatorOrders, &latestState)
				singleStateTx <- StateMsg{id, latestState}

				drv_newOrder <- first_element // Send the first element of the elevatorOrders to the driver

				unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
			}

		case a := <-consumer1drv_floors: // REACHING FLOOR
			lastFloor = a // Update the last floor

			// Update & send the new state of the elevator to the master

			updateState(&d, lastFloor, elevatorOrders, &latestState)
			singleStateTx <- StateMsg{id, latestState}

		case a := <-drv_stop: // STOP BUTTON
			switch {
			case a:
				// Rising edge, from unpressed to pressed
				lockMutexes(&mutex_d)

				// Stop the elevator
				elevio.SetStopLamp(true)
				lastDirForStopFunction = d // Save the last direction before stopping, ## PLACEHOLDER ##
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

				// The master adds himself to the activeElevators list and sends it to the other elevators
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

		case a := <-drv_obstr: // OBSTRUCTION
			// Unable to close the doors until obstruction switch is released
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

		case p := <-peerUpdateCh: // PEER UPDATE
			// Convert the Peers, New & Lost from strings to structures
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

				// The master updates the activeElevators array and sends it to the other elevators
				if currentRole == "Master" {
					mutex_activeElevators.Lock()
					alreadyExists := isElevatorActive(mNew.Id) // Check if the elevator is already active

					if !alreadyExists {
						activeElevators = append(activeElevators, mNew.Id) // Add the elevator to the activeElevators list
					}

					activeElevators = sortElevators(activeElevators) // Sort for the mapping to remain correct (see communication.go)
					mutex_activeElevators.Unlock()

					activeElevatorsChannelTx <- activeElevators // Send the activeElevators list to the other elevators
				}

			case len(mLost) > 0: // A peer leaves the network

				lostElevator := mLost[0] // We assume that we only have one down elevator at a time

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
						go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx, backupStatesTx, newStatesRx, hallOrderCompletedTx)
						newStatesTx <- backupStates // Sending the backupStates to the new master

					}
				case "PrimaryBackup": // The PrimaryBackup leaves the network
					// The Regular becomes PrimaryBackup
					if currentRole == "Regular" {
						newRole = "PrimaryBackup"
						go PrimaryBackupRoutine(backupStatesRx)
					}
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
		case a := <-hallOrderCompletedRx:
			turnOffHallLights(a...)
		}

	}

}
