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

const (
	BTN_PORT   = 16164
	STR_PORT   = 16569
	STATE_PORT = 16165
)

const numFloors = 4

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

var mutex_d sync.Mutex // Mutex for the direction of the elevator

var lastDirForStopFunction elevio.MotorDirection // The last direction the elevator was moving in before the stop button was pressed

type ElevatorState struct {
}

type ElevState struct {
	elevatorOrders    []Order
	d                 elevio.MotorDirection
	position          [2*numFloors - 1]bool
	doorOpen          bool
	stopButtonPressed bool
	id                string
}

type HallOrderMessage struct {
	order Order
	id    string
}

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
	port_val := flag.String("port", "", "The port of the elevator client / server")
	role_val := flag.String("role", "", "The role of the elevator")
	id_val := flag.String("id", "", "The id of the elevator")
	flag.Parse()

	port := *port_val
	fmt.Printf("Working on address: %v\n", "localhost:"+port)
	role = *role_val
	fmt.Printf("Role passed: %v\n", role)
	id := *id_val
	fmt.Printf("Id passed: %v\n", id)

	// Section_END -- FLAGS

	// Section_START -- NETWORK CHANNELS (For all)

	peerUpdateCh := make(chan peers.PeerUpdate)     // Creates a channel that listens for updates from the peers (New, Loss)
	peerTxEnable := make(chan bool)                 // Creates a channel that enables/disables the transmitter
	go peers.Transmitter(15647, role, peerTxEnable) // Creates a channel that broadcasts our role
	go peers.Receiver(15647, peerUpdateCh)          // Creates a channel that listens

	// We make channels for sending and receiving strings (confirmations, etc.)
	helloTx := make(chan string)
	helloRx := make(chan string)

	go bcast.Transmitter(STR_PORT, helloTx)
	go bcast.Receiver(STR_PORT, helloRx)

	// Making a channel for transmitting hallButtons
	hallBtnTx := make(chan elevio.ButtonEvent)
	go bcast.Transmitter(BTN_PORT, hallBtnTx)

	// Making a channel for recieving elevator states
	stateTx := make(chan HallOrderMessage)
	go bcast.Transmitter(STATE_PORT, stateTx)

	// Making a channel for recieving orders from the master
	orderRx := make(chan Order)
	go bcast.Receiver(BTN_PORT, orderRx)

	// Section_END - NETWORK CHANNELS (For all)

	// Section_START -- NETWORK CHANNELS (For specific roles)

	// Channels to receive & send elev. states to the master elevator
	switch role {
	case "Master":
		// Making a channel for recieving hallbuttons
		hallBtnRx := make(chan elevio.ButtonEvent)
		go bcast.Receiver(BTN_PORT, hallBtnRx)

		// Making a channel for recieving elevator states
		stateRx := make(chan ElevState)
		go bcast.Receiver(STATE_PORT, stateRx)

		// Making a channel for sending orders to the slaves
		orderTx := make(chan Order)
		go bcast.Transmitter(BTN_PORT, orderTx)

		// Starting the master routine
		go MasterRoutine(hallBtnTx, hallBtnRx, stateRx, orderTx)

	case "Primary":
		go PrimaryRoutine()
	}

	// Section_END -- NETWORK CHANNELS (For specific roles)

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
		drv_finishedInitialization <- true
	}()

	<-drv_finishedInitialization

	fmt.Printf("Initialization finished\n")
	helloTx <- "Initialization finished.\n"

	// Section_END ---- Initialization

	// Starting the goroutines for tracking the position of the elevator & attending to specific orders
	go trackPosition(drv_floors2, drv_DirectionChange, &d) // Starts tracking the position of the elevator
	go attendToSpecificOrder(&d, drv_floors, drv_newOrder, drv_DirectionChange)

	for { // Main loop
		select { // Select statement

		case a := <-drv_buttons: // New button update
			// Gets a new button press. If it's a hall order, forwards it to the master

			time.Sleep(30 * time.Millisecond) // Poll rate of the buttons

			lockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

			switch {
			case a.Button == elevio.BT_HallUp || a.Button == elevio.BT_HallDown: // If it's a hall order
				hallBtnTx <- a // Send it
				fmt.Print("\nReceived hall press & forwarded it.\n")
			case a.Button == elevio.BT_Cab: // Else (it's a cab)
				addOrder(a.Floor, 0, cab) // Add it to the local array
				fmt.Printf("\nAdded cab order, current direction is now: %v\n", d)
				fmt.Printf("Added cab order, elevatorOrders is now: %v\n", elevatorOrders)
				fmt.Printf("Added cab order, positionArray is now: %v\n", posArray)
			}

			// Sort the local orders of the elevator
			sortAllOrders(&elevatorOrders, d, posArray)

			first_element := elevatorOrders[0]

			fmt.Printf("ElevatorOrders for %v is now: %v\n", role, elevatorOrders)

			// Sending the first element of elevatorOrders through the drv_newOrder channel
			// We don't have to worry about the possibility of it being the same order that we are attending to
			// This is because we only set the current direction to the same as it was
			unlockMutexes(&mutex_elevatorOrders, &mutex_d, &mutex_posArray)

			drv_newOrder <- first_element

		case a := <-drv_floors: // Reaching a new floor
			lastFloor = a

		case a := <-helloRx: // Received a string message from another elevator
			fmt.Printf("Received: %#v\n", a)

		case a := <-drv_stop: // Stop button pressed
			switch {
			case a:
				// Rising edge, from unpressed to pressed
				fmt.Printf("Received rising edge from drv_stop\n")
				lockMutexes(&mutex_d)
				elevio.SetStopLamp(true)
				lastDirForStopFunction = d
				elevio.SetMotorDirection(elevio.MD_Stop)
				unlockMutexes(&mutex_d)

			case !a:
				// Falling edge, from pressed to unpressed
				fmt.Printf("Received falling edge from drv_stop\n")
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
