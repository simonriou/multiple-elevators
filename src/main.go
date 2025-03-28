package main

import (
	"Driver-go/elevio"
	"Network-go/network/bcast"
	"Network-go/network/peers"
	"context"
)

func main() {
	// Section_START -- FLAGS & ROLE
	port, initialRole, id := getFlags()
	roleChannel := make(chan string)
	// Section_END -- FLAGS

	currentRole := initialRole

	// Section_START -- NETWORK INITIALIZATION
	peerUpdateCh := make(chan peers.PeerUpdate)                           // Updates from peers
	peerTxEnable := make(chan bool)                                       // Enables/disables the transmitter
	go peers.Transmitter(PeerChannel_PORT, id, roleChannel, peerTxEnable) // Broadcast role
	roleChannel <- currentRole
	go peers.Receiver(PeerChannel_PORT, peerUpdateCh) // Listen for updates

	// Check if the ID of the elevator is valid

	// Section_END -- NETWORK INITIALIZATION

	// Section_START -- CHANNELS
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
	localStatesForCabOrders := make(chan StateMsg) // ALL - Turn off cab lights after completing order
	selfUpdate := make(chan StateMsg)              // ALL - Check for updates of the state to prevent loosing the elevator

	drv_buttons_forCabLights := make(chan elevio.ButtonEvent, 100)
	drv_buttons_forOrderHandling := make(chan elevio.ButtonEvent, 100)
	go relayDrvButtons(drv_buttons, drv_buttons_forCabLights, drv_buttons_forOrderHandling)

	go elevio.PollButtons(drv_buttons)         // Button updates
	go elevio.PollFloorSensor(drv_floors)      // Floors updates
	go elevio.PollFloorSensor2(drv_floors2)    // Floors updates (for tracking position)
	go elevio.PollObstructionSwitch(drv_obstr) // Obstruction updates
	go elevio.PollStopButton(drv_stop)         // Stop button presses

	// Channels for the network
	hallBtnTx := make(chan elevio.ButtonEvent)       // ALL - Send hall orders to the master
	hallOrderRx := make(chan HallOrderMsg)           // ALL - Receive hall orders from the master
	singleStateTx := make(chan StateMsg)             // ALL - Send the state of the elevator to the master
	hallOrderCompletedLightsRx := make(chan []Order) // ALL - Confirm hall order (for lights)
	activeElevatorsChannelTx := make(chan []int)     // ALL - The channel on which we send the active elevators list
	activeElevatorsChannelRx := make(chan []int)     // ALL - The channel on which we receive the active elevators list
	retrieveCabOrdersRx := make(chan CabOrderMsg)    // ALL - Retrieve the cab orders from the master
	askForCabOrdersTx := make(chan int)              // ALL - Ask for the cab orders from the master

	allStatesFromMasterRx := make(chan [numElev]ElevState) // ALL - Receive all states from the master
	singleStateFromSlaveTx := make(chan StateMsg)          // ALL - Send the state of the elevator to the master

	go bcast.Receiver(HallOrder_PORT, hallOrderRx)
	go bcast.Transmitter(HallOrderRawBTN_PORT, hallBtnTx)
	go bcast.Transmitter(SingleElevatorState_PORT, singleStateTx)
	go bcast.Receiver(HallOrderCompleted_PORT, hallOrderCompletedLightsRx)
	go bcast.Receiver(ActiveElevators_PORT, activeElevatorsChannelRx)
	go bcast.Transmitter(ActiveElevators_PORT, activeElevatorsChannelTx)
	go bcast.Receiver(RetrieveCabOrders_PORT, retrieveCabOrdersRx)
	go bcast.Transmitter(AskForCabOrders_PORT, askForCabOrdersTx)
	go bcast.Receiver(SpamFromMaster_PORT, allStatesFromMasterRx)
	go bcast.Transmitter(SpamFromSlave_PORT, singleStateFromSlaveTx)

	go forwarderStateMsg(singleStateTx, selfUpdate)

	// Channels for specific roles
	hallBtnRx := make(chan elevio.ButtonEvent)      // MASTER - Receive hall orders from slaves
	hallOrderTx := make(chan HallOrderMsg)          // MASTER - Send hall orders to slaves
	singleStateRx := make(chan StateMsg)            // MASTER - Receive states from slaves
	backupStatesRx := make(chan [numElev]ElevState) // BACKUP - Receive all states from master
	backupStatesTx := make(chan [numElev]ElevState) // MASTER - Send all states to backup
	newStatesRx := make(chan [numElev]ElevState)    // MASTER - Receive all NEW states from backup
	newStatesTx := make(chan [numElev]ElevState)    // BACKUP - Send all states to the NEW master
	hallOrderCompletedTx := make(chan []Order)      // Master - Send completed hallorder(s) to single elevators
	retrieveCabOrdersTx := make(chan CabOrderMsg)   // ALL - Retrieve the cab orders from the master
	askForCabOrdersRx := make(chan int)             // ALL - Ask for the cab orders from the master

	allStatesFromMasterTx := make(chan [numElev]ElevState) // ALL - Send all states to the master
	singleStateFromSlaveRx := make(chan StateMsg)          // ALL - Receive the state of the elevator from the master

	go bcast.Transmitter(BackupStates_PORT, newStatesTx) // LOCAL - Used to send the states to the NEW master (used in role changes)

	_ = hallBtnRx
	_ = hallOrderTx
	_ = singleStateRx
	_ = backupStatesTx
	_ = newStatesRx
	_ = retrieveCabOrdersTx
	_ = askForCabOrdersRx
	_ = allStatesFromMasterTx
	_ = singleStateFromSlaveRx

	// Section_END -- CHANNELS

	// Create a context for stopping unwanted master routines
	ctx, cancel := context.WithCancel(context.Background())
	_ = ctx
	_ = cancel

	askForCabOrdersTx <- id // Ask for the cab orders from the master

	// Section_START -- ROLES-SPECIFIC ACTIONS
	switch role {
	case "Master":

		activeElevators = append(activeElevators, id) // Add the master to the activeElevators list

		// Starting the Master Routine
		go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx, backupStatesTx, newStatesRx, hallOrderCompletedTx,
			retrieveCabOrdersTx, askForCabOrdersRx, ctx, hallBtnTx, activeElevatorsChannelTx, allStatesFromMasterTx,
			singleStateFromSlaveRx)

		// This is the initial states of the elevators
		var allStates [numElev]ElevState
		allStates = initAllStates(allStates)

		// Send the initial states to the master
		newStatesRx <- allStates

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
	consumer3drv_floors := make(chan int)
	go relayDrvFloors(drv_floors, consumer1drv_floors, consumer2drv_floors, consumer3drv_floors)

	d = elevio.MD_Stop // Update d so that states are accurate

	// Send the initial state of the elevator to the master
	singleStateTx <- StateMsg{id, latestState}

	// Starting the goroutines for tracking the position of the elevator & attending to specific orders
	go trackPosition(drv_floors2, drv_DirectionChange, &d) // Starts tracking the position of the elevator
	go attendToSpecificOrder(&d, consumer2drv_floors, drv_newOrder, drv_DirectionChange, singleStateTx, id, localStatesForCabOrders, hallBtnTx)

	// Section_START -- RERTIEVE CAB ORDERS
	// We send our ID to the master to ask for the cab orders
	askForCabOrdersTx <- id
	// Secton_END -- RETRIEVE CAB ORDERS

	updateState(&d, lastFloor, elevatorOrders, &latestState)
	singleStateTx <- StateMsg{id, latestState}

	// Section_END -- LOCAL INITIALIZATION

	go handleFloorLights(consumer3drv_floors)
	go handleObstruction(drv_obstr)                                                                    // Listens to the obstruction button
	go handleElevatorUpdate(activeElevatorsChannelRx)                                                  // Listens to active elevators updates
	go handleButtonPress(drv_buttons_forOrderHandling, hallBtnTx, &d, singleStateTx, id, drv_newOrder) // Listens to new button presses
	go handleNewFloorReached(consumer1drv_floors, &d, singleStateTx, id)                               // Listens to floor updates
	go handleNewHallOrder(hallOrderRx, id, &d, singleStateTx, drv_newOrder, hallOrderCompletedTx)      // Listens to new orders from the master
	go handlePeerUpdate(peerUpdateCh, currentRole, activeElevatorsChannelTx, backupStatesRx,
		hallBtnRx, singleStateRx, hallOrderTx, backupStatesTx, newStatesRx, hallOrderCompletedTx,
		retrieveCabOrdersTx, askForCabOrdersRx, newStatesTx, roleChannel, hallBtnTx, id, ctx, cancel,
		allStatesFromMasterTx, singleStateFromSlaveRx) // Listens to peer updates on the network
	go handleTurnOffLightsHallOrderCompleted(hallOrderCompletedLightsRx) // Listens for completed hall orders
	go handleTurnOffLightsCabOrderCompleted(localStatesForCabOrders)
	go handleTurnOnLightsCabOrder(drv_buttons_forCabLights)
	go handleRetrieveCab(retrieveCabOrdersRx, id, &d, singleStateTx, drv_newOrder) // Listens for cab order retrieving
	go handleStopButton(drv_stop, &d, id, activeElevatorsChannelTx, hallBtnTx)     // Listens for stop button presses

	go receiveSpamFromMaster(allStatesFromMasterRx, id)
	go spamMaster(singleStateFromSlaveTx, id) // Sends the state of the elevator to the master periodically

	select {}
}
