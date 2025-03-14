package main

import (
	"Driver-go/elevio"
	"Network-go/network/bcast"
	"Network-go/network/peers"
	"flag"
	"fmt"
	"sync"
	"time"
)

const ( // Ports
	HallOrder_PORT = 16120 + iota
	HallOrderRawBTN_PORT
	SingleElevatorState_PORT
	AllStates_PORT
	PeerChannel_PORT
)

const numFloors = 4 // Number of floors
const numElev = 1   // Number of elevators

type ElevState struct { // Struct for the state of the elevator
	Behavior      string  // 'moving' or 'idle'
	Floor         int     // The floor the elevator is at
	Direction     string  // 'up', 'down' or 'stop'
	LocalRequests []Order // The requests of the elevator
}

type StateMsg struct { // Structure used to send our state to the master
	Id    int
	State ElevState
}

var (
	elevatorOrders       []Order // The local orders array for this elevator
	mutex_elevatorOrders sync.Mutex
)

var (
	posArray       [2*numFloors - 1]bool // The position array for this elevator
	mutex_posArray sync.Mutex
)

var (
	ableToCloseDoors bool // A boolean that tells us if we are able to close the doors
	mutex_doors      sync.Mutex
)

var (
	role string // The role of the elevator (Master, Slave or PrimaryBackup)
)

var (
	lastFloor int // The last floor the elevator was at
)

var (
	latestState ElevState // The latest state of the elevator
	mutex_state sync.Mutex
)

var mutex_d sync.Mutex // Mutex for the direction of the elevator

var lastDirForStopFunction elevio.MotorDirection // The last direction the elevator was moving in before the stop button was pressed

func lockMutexes(mutexes ...*sync.Mutex) { // Locks multiple mutexes
	for _, m := range mutexes {
		m.Lock()
	}
}

func unlockMutexes(mutexes ...*sync.Mutex) { // Unlocks multiple mutexes
	for _, m := range mutexes {
		m.Unlock()
	}
}

func initSingleElev(d elevio.MotorDirection, drv_floors chan int) {
	drv_finishedInitialization := make(chan bool)
	go func() {
		elevio.SetMotorDirection(d)
		for {
			a := <-drv_floors
			fmt.Printf("%v\n",a)
			if a == 0 {
				d = elevio.MD_Stop
				elevio.SetMotorDirection(d)
				break
			}
		}
		fmt.Println("Found 0 floor")
		ableToCloseDoors = true
		turnOffLights(Order{0, -1, 0}, true)

		drv_finishedInitialization <- true
	}()

	<-drv_finishedInitialization

	fmt.Printf("Initialization finished\n")
}

func main() {
	// Single elevator

		// Section_START -- FLAGS
		// Decide the port on which we are working (for the server) & the role of the elevator
		port_raw := flag.String("port", "", "The port of the elevator client / server")
		role_raw := flag.String("role", "", "The role of the elevator")
		id_raw := flag.Int("id", -1, "The id of the elevator")
		flag.Parse()

		port := *port_raw
		role = *role_raw
		id := *id_raw
		fmt.Printf("Working on address: %v\n", "localhost:"+port)
		fmt.Printf("Role passed: %v\n", role)
		fmt.Printf("Id passed: %v\n", id)
		// Section_END -- FLAGS

		// Make functionality for peer-updates
		peerUpdateCh := make(chan peers.PeerUpdate)           // Updates from peers
		peerTxEnable := make(chan bool)                       // Enables/disables the transmitter
		go peers.Transmitter(PeerChannel_PORT, role, peerTxEnable) // Broadcast role
		go peers.Receiver(PeerChannel_PORT, peerUpdateCh)          // Listen for updates

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

		go bcast.Receiver(HallOrder_PORT, hallOrderRx)
		go bcast.Transmitter(HallOrderRawBTN_PORT, hallBtnTx)
		go bcast.Transmitter(SingleElevatorState_PORT, singleStateTx)

		hallBtnRx := make(chan elevio.ButtonEvent)
		hallOrderTx := make(chan HallOrderMsg)
		singleStateRx := make(chan StateMsg)
		AllStatesRx := make(chan [numElev]ElevState)
		AllStatesTx := make(chan [numElev]ElevState)

		_= hallBtnRx 
		_= hallOrderTx 
		_= singleStateRx 
		_= AllStatesRx 
		_= AllStatesTx 

		switch role {
			case "Master":
				go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx, AllStatesTx)
			case "PrimaryBackup":
		}

		var d elevio.MotorDirection = elevio.MD_Down
		
		initSingleElev(d, drv_floors)
		consumer1drv_floors := make(chan int)
		consumer2drv_floors := make(chan int)
		go relay(drv_floors, consumer1drv_floors, consumer2drv_floors)
		

		updateState(&d, 0, elevatorOrders, &latestState)
		singleStateTx <- StateMsg{id, latestState}


		// Starting the goroutines for tracking the position of the elevator & attending to specific orders
		go trackPosition(drv_floors2, drv_DirectionChange, &d) // Starts tracking the position of the elevator
		go attendToSpecificOrder(&d, consumer2drv_floors, drv_newOrder, drv_DirectionChange)

		
		for { // Main loop
			select { // Select statement
	
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
					singleStateTx <- StateMsg{id, latestState}
	
					drv_newOrder <- first_element
				}
	
				unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
	
			case a := <-hallOrderRx: // Received an HallOrderMsg from the master
	
				// Handle the hallOrder if the id's match
				if a.Id == id {
		
					newHallOrder := a.HallOrder
					fmt.Printf("The new hallOrder is now: %v\n", newHallOrder)

					lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
	
					addOrder(newHallOrder.Floor, newHallOrder.Direction, hall)
					sortAllOrders(&elevatorOrders, d, posArray)
					first_element := elevatorOrders[0]
					fmt.Printf("ElevatorOrders after sorting: %v\n", elevatorOrders)
	
					// Send the new state of the elevator to the master
					updateState(&d, lastFloor, elevatorOrders, &latestState)
					singleStateTx <- StateMsg{id, latestState}
					
					drv_newOrder <- first_element
	
					unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
				}
	
			case a := <-consumer1drv_floors: // Reaching a new floor
				fmt.Println("main new floor: ", a)
				lastFloor = a
	
				// Send new state
				updateState(&d, lastFloor, elevatorOrders, &latestState)
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
	
				case !a:
					// Falling edge, from pressed to unpressed
					lockMutexes(&mutex_d)
					elevio.SetMotorDirection(lastDirForStopFunction)
					unlockMutexes(&mutex_d)
	
					elevio.SetStopLamp(false)
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
				fmt.Printf("Peer update:\n")
				fmt.Printf("  Peers:    %q\n", p.Peers)
				fmt.Printf("  New:      %q\n", p.New)
				fmt.Printf("  Lost:     %q\n", p.Lost)
			}
	
		}

}
