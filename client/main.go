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
	AllStatesPort
	PeerChannel
)

const numFloors = 4 // Number of floors
const numElev = 3   // Number of elevators

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

func main() {
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


	// Create the network channels for at the single-Elevator end
	hallBtnTx := make(chan elevio.ButtonEvent)
	hallOrderRx := make(chan Order)
	singleStateTx := make(chan StateMsg)

	// Initialize network (in communications.go)
	InitializeNetwork(role, id, hallOrderRx, hallBtnTx, singleStateTx)

	// Initialize the elevator
	elevio.Init("localhost:"+port, numFloors)

	// Channels for the driver
	drv_buttons := make(chan elevio.ButtonEvent)
	drv_floors := make(chan int)
	drv_floors2 := make(chan int)
	drv_floors3 := make(chan int)
	drv_obstr := make(chan bool)
	drv_stop := make(chan bool)
	drv_newOrder := make(chan Order)
	drv_DirectionChange := make(chan elevio.MotorDirection)
	drv_finishedInitialization := make(chan bool)

	go elevio.PollButtons(drv_buttons)         // Starts checking for button updates
	go elevio.PollFloorSensor(drv_floors)      // Starts checking for floors updates
	go elevio.PollFloorSensor2(drv_floors2)    // Starts checking for floors updates (for tracking position)
	go elevio.PollFloorSensor(drv_floors3)     // Starts checking for floors updates (for safety measures)
	go elevio.PollObstructionSwitch(drv_obstr) // Starts checking for obstruction updates
	go elevio.PollStopButton(drv_stop)         // Starts checking for stop button presses

	var d elevio.MotorDirection = elevio.MD_Down

	// Section_START ---- Initialization

	go func() {
		elevio.SetMotorDirection(d)
		for {
			a := <-drv_floors
			if a == 0 {
				d = elevio.MD_Stop
				elevio.SetMotorDirection(d)
				break
			}
		}
		ableToCloseDoors = true
		turnOffLights(Order{0, -1, 0}, true)

		updateState(&d, 0, elevatorOrders, &latestState)

		drv_finishedInitialization <- true
	}()

	<-drv_finishedInitialization

	fmt.Printf("Initialization finished\n")

	// Section_END ---- Initialization

	// Starting the goroutines for tracking the position of the elevator & attending to specific orders
	go trackPosition(drv_floors2, drv_DirectionChange, &d) // Starts tracking the position of the elevator
	go attendToSpecificOrder(&d, drv_floors, drv_newOrder, drv_DirectionChange)

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

		case a := <-hallOrderRx: // Received an hallOrderAndId from the master
			// Assumtion: Every single-Elevator will recieve this
			if a.id == id {
				newHallOrder := a.hallOrder
				lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

				addOrder(newHallOrder.Floor, newHallOrder.direction, hall) 
				sortAllOrders(&elevatorOrders, d, posArray)
				first_element := elevatorOrders[0]

				// Send the new state of the elevator to the master
				updateState(&d, lastFloor, elevatorOrders, &latestState)
				singleStateTx <- StateMsg{id, latestState}

				drv_newOrder <- first_element

				unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)
			}

		case a := <-drv_floors: // Reaching a new floor
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
		}

	}
}
