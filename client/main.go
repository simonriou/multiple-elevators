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
	// Section_START -- FLAGS
	port, role, id := getFlags()
	// Section_END -- FLAGS

	// Section_START -- CHANNELS
	peerUpdateCh := make(chan peers.PeerUpdate)                    // Updates from peers
	peerTxEnable := make(chan bool)                                // Enables/disables the transmitter
	go peers.Transmitter(PeerChannel_PORT, id, role, peerTxEnable) // Broadcast role
	// go peers.Transmitter(PeerChannel_PORT, role, peerTxEnable) // Broadcast role
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

	go elevio.PollButtons(drv_buttons)         // Starts checking for button updates
	go elevio.PollFloorSensor(drv_floors)      // Starts checking for floors updates
	go elevio.PollFloorSensor2(drv_floors2)    // Starts checking for floors updates (for tracking position)
	go elevio.PollObstructionSwitch(drv_obstr) // Starts checking for obstruction updates
	go elevio.PollStopButton(drv_stop)         // Starts checking for stop button presses

	hallBtnTx := make(chan elevio.ButtonEvent)
	hallOrderRx := make(chan HallOrderMsg)
	singleStateTx := make(chan StateMsg)
	hallOrderCompleted := make(chan Order)

	go bcast.Receiver(HallOrder_PORT, hallOrderRx)
	go bcast.Transmitter(HallOrderRawBTN_PORT, hallBtnTx)
	go bcast.Transmitter(SingleElevatorState_PORT, singleStateTx)
	go bcast.Receiver(hallOrderCompleted_PORT, hallOrderCompleted)

	hallBtnRx := make(chan elevio.ButtonEvent)
	hallOrderTx := make(chan HallOrderMsg)
	singleStateRx := make(chan StateMsg)
	backupStatesRx := make(chan [numElev]ElevState) // Receive all states from master to backup
	backupStatesTx := make(chan [numElev]ElevState) // Send all states from master to backup
	newStatesRx := make(chan [numElev]ElevState)    // Receive all NEW states from backup to master
	newStatesTx := make(chan [numElev]ElevState)    // Send all NEW states from backup to master

	go bcast.Transmitter(BackupStates_PORT, newStatesTx) // Used to send the states to the NEW master (used in role changes)

	_ = hallBtnRx
	_ = hallOrderTx
	_ = singleStateRx
	_ = backupStatesTx
	_ = backupStatesRx
	_ = newStatesRx

	// Section_END -- CHANNELS

	// Section_START -- ROLES-SPECIFIC ACTIONS
	switch role {
	case "Master":
		// We assume that all elevators are active
		for i := 0; i < numElev; i++ {
			activeElevators = append(activeElevators, i)
		}

		go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx, backupStatesTx, newStatesRx, hallOrderCompleted)

		var allStates [numElev]ElevState
		uninitializedOrderArray := []Order{
			{
				Floor:     0,
				Direction: up,   // Replace with your OrderDirection constant (e.g., Up or Down)
				OrderType: hall, // Replace with your OrderType constant (e.g., Hall or Cab)
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
		backupStatesRx <- allStates
	case "PrimaryBackup":
		go PrimaryBackupRoutine(backupStatesRx)
	}
	// Section_END -- ROLES-SPECIFIC ACTIONS

	// Section_START -- LOCAL INITIALIZATION
	var d elevio.MotorDirection = elevio.MD_Down

	initSingleElev(d, drv_floors)
	consumer1drv_floors := make(chan int)
	consumer2drv_floors := make(chan int)
	go relay(drv_floors, consumer1drv_floors, consumer2drv_floors)

	d = elevio.MD_Stop
	updateState(&d, 0, elevatorOrders, &latestState)
	fmt.Printf("SingleStateTx sent over the network from id: %v, the latest state: %v\n", id, latestState)
	singleStateTx <- StateMsg{id, latestState}

	// Starting the goroutines for tracking the position of the elevator & attending to specific orders
	go trackPosition(drv_floors2, drv_DirectionChange, &d) // Starts tracking the position of the elevator
	go attendToSpecificOrder(&d, consumer2drv_floors, drv_newOrder, drv_DirectionChange)
	// Section_END -- LOCAL INITIALIZATION

	for { // MAIN LOOP
		select {

		case a := <-drv_buttons: // New button update
			// Gets a new button press. If it's a hall order, forwards it to the master

			time.Sleep(30 * time.Millisecond) // > Poll rate of the buttons

			lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

			switch {
			case a.Button == elevio.BT_HallUp || a.Button == elevio.BT_HallDown: // If it's a hall order

				hallBtnTx <- a

			case a.Button == elevio.BT_Cab: // Else (it's a cab)

				addOrder(a.Floor, 0, cab)
				sortAllOrders(&elevatorOrders, d, posArray)
				first_element := elevatorOrders[0]

				// Send the new state of the elevator to the master
				updateState(&d, lastFloor, elevatorOrders, &latestState)
				fmt.Printf("SingleStateTx sent over the network from id: %v, the latest state: %v\n", id, latestState)
				singleStateTx <- StateMsg{id, latestState}

				drv_newOrder <- first_element
			}

			unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

		case a := <-hallOrderRx: // Received an HallOrderMsg from the master
			// We turn up the lights on all slaves' servers
			// elevio.SetButtonLamp(elevio.ButtonType(a.HallOrder.Direction), a.HallOrder.Floor, true)

			// Handle the hallOrder if the id's match
			if a.Id == id {

				newHallOrder := a.HallOrder

				lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

				addOrder(newHallOrder.Floor, newHallOrder.Direction, hall)
				sortAllOrders(&elevatorOrders, d, posArray)
				first_element := elevatorOrders[0]

				// Send the new state of the elevator to the master
				updateState(&d, lastFloor, elevatorOrders, &latestState)
				fmt.Printf("SingleStateTx sent over the network from id: %v, the latest state: %v\n", id, latestState)
				singleStateTx <- StateMsg{id, latestState}

				drv_newOrder <- first_element

				unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
			}

		case a := <-consumer1drv_floors: // Reaching a new floor
			lastFloor = a

			// Send new state
			updateState(&d, lastFloor, elevatorOrders, &latestState)
			fmt.Printf("SingleStateTx sent over the network from id: %v, the latest state: %v\n", id, latestState)
			singleStateTx <- StateMsg{id, latestState}

		case a := <-drv_stop: // Stop button pressed
			switch {
			case a:
				// Rising edge, from unpressed to pressed
				lockMutexes(&mutex_d)
				elevio.SetStopLamp(true)
				lastDirForStopFunction = d
				elevio.SetMotorDirection(elevio.MD_Stop)
				unlockMutexes(&mutex_d)

				// Remove the elevator from the activeElevators list
				alreadyExists := isElevatorActive(id)
				if alreadyExists {
					lockMutexes(&mutex_activeElevators)
					removeElevator(id)
					unlockMutexes(&mutex_activeElevators)
				}

			case !a:
				// Falling edge, from pressed to unpressed
				lockMutexes(&mutex_d)
				elevio.SetMotorDirection(lastDirForStopFunction)
				unlockMutexes(&mutex_d)

				elevio.SetStopLamp(false)

				// Adds the elevator to the activeElevators list
				alreadyExists := isElevatorActive(id)
				if !alreadyExists {
					lockMutexes(&mutex_activeElevators)
					activeElevators = append(activeElevators, id)
					unlockMutexes(&mutex_activeElevators)
				}
			}

		case a := <-drv_obstr: // Obstruction switch pressed (meaning doors are opened)
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

		case p := <-peerUpdateCh:
			// Convert the p.Peers, p.New and p.Lost from string to structures
			var mPeers []peers.ElevIdentity
			for _, v := range p.Peers {
				mPeers = append(mPeers, stringToElevIdentity(v))
			}

			var mNew peers.ElevIdentity
			if p.New != "" {
				mNew = stringToElevIdentity(p.New)
			}
			var mLost []peers.ElevIdentity
			for _, v := range p.Lost {
				mLost = append(mLost, stringToElevIdentity(v))
			}

			// Display the peer update
			fmt.Printf("Peer update:\n")
			fmt.Printf("  Peers:    %v\n", mPeers)
			fmt.Printf("  New:      %v\n", mNew)
			fmt.Printf("  Lost:     %v\n", mLost)

			fmt.Printf("Number of lost elevators: %v\n", len(mLost))

			switch {
			case mNew != (peers.ElevIdentity{}): // A new peer joins the network

				// Check if the new peer already is in the activeElevators list
				mutex_activeElevators.Lock()
				alreadyExists := isElevatorActive(mNew.Id)

				if !alreadyExists {
					activeElevators = append(activeElevators, mNew.Id)
				}
				activeElevators = sortElevators(activeElevators)
				mutex_activeElevators.Unlock()

				fmt.Printf("Active elevators: %v\n", activeElevators)

			case len(mLost) > 0: // A peer leaves the network
				fmt.Print("LOST ELEVATOR\n")
				// We assume that we only have one down elevator at a time
				lostElevator := mLost[0]

				alreadyExists := isElevatorActive(lostElevator.Id)
				if alreadyExists { // If the elevator is active
					removeElevator(lostElevator.Id) // Remove the elevator from the activeElevators list
				}
				activeElevators = sortElevators(activeElevators)
				fmt.Printf("Active elevators: %v\n", activeElevators)

				// Handle the roles change when a peer leaves
				switch lostElevator.Role {
				case "Master":
					// The master leaves the network. The Regular becomes PrimaryBackup & the PrimaryBackup becomes Master
					switch role {
					case "Regular":

						// Switch role to primary backup and launch it
						role = "PrimaryBackup"
						go PrimaryBackupRoutine(backupStatesRx)

					case "PrimaryBackup":

						role = "Master"
						go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx, backupStatesTx, newStatesRx, hallOrderCompleted)
						newStatesTx <- backupStates // Sending the backupStates to the new master

					}
				case "PrimaryBackup":
					// The PrimaryBackup leaves the network. The Regular becomes PrimaryBackup
					if role == "Regular" {
						role = "PrimaryBackup"
						go PrimaryBackupRoutine(backupStatesRx)
					}
				}

			}

		}

	}

}
