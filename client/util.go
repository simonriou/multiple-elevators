package main

import (
	"Driver-go/elevio"
	"Network-go/network/peers"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

func sortElevators(activeElevators []int) []int {
	// Sort the active elevators
	for i := 0; i < len(activeElevators); i++ {
		for j := i + 1; j < len(activeElevators); j++ {
			if activeElevators[i] > activeElevators[j] {
				activeElevators[i], activeElevators[j] = activeElevators[j], activeElevators[i]
			}
		}
	}
	return activeElevators
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

func isElevatorActive(elevatorId int) bool {
	// Check if the elevator is active
	for _, id := range activeElevators {
		if id == elevatorId {
			return true
		}
	}
	return false
}

func removeElevator(elevatorId int) {
	// Removes the id of the elevator from the list of active elevators
	for i, id := range activeElevators {
		if id == elevatorId {
			activeElevators = append(activeElevators[:i], activeElevators[i+1:]...)
			break // Exit to avoid issues with changed indices (works because ids are unique)
		}
	}
}

func stringToElevIdentity(data string) peers.ElevIdentity { // Convert a string to an ElevIdentity struct
	var msg peers.ElevIdentity
	err := json.Unmarshal([]byte(data), &msg)
	if err != nil {
		fmt.Println("Error: ", err)
	}
	return msg
}

func btnPressToOrder(btn elevio.ButtonEvent) Order { // Convert a button press to an order for hall orders
	orderType := hall
	orderDirection := up
	if btn.Button == elevio.BT_HallDown {
		orderDirection = down
	}
	return Order{Floor: btn.Floor, Direction: orderDirection, OrderType: orderType}
}

func elevDirectionToElevioButtonType(Direction OrderDirection) (buttonType elevio.ButtonType) {
	// 1 for up, -1 for down
	/* const (
		BT_HallUp   ButtonType = 0
		BT_HallDown            = 1
		BT_Cab                 = 2
	) */
	switch Direction {
	case 1:
		return elevio.BT_HallUp
	case -1:
		return elevio.BT_HallDown
	default:
		fmt.Printf("Error: invalid direction was passed to function elevDirectionToElevioButtonType\n")
	}

	return
}

func determineBehaviour(d *elevio.MotorDirection) string { // Determine the behaviour of the elevator based on its direction
	switch {
	case *d == elevio.MD_Stop:
		return "idle"
	case *d == elevio.MD_Up || *d == elevio.MD_Down:
		return "moving"
	}
	return "unknown"
}

func motorDirectionToString(d elevio.MotorDirection) string { // Convert the motor direction to a string
	switch {
	case d == elevio.MD_Up:
		return "up"
	case d == elevio.MD_Down:
		return "down"
	case d == elevio.MD_Stop:
		return "stop"
	}
	return "unknown"
}

func updateState(d *elevio.MotorDirection, lastFloor int, elevatorOrders []Order, latestState *ElevState) { // Update the state of the elevator
	mutex_state.Lock()
	defer mutex_state.Unlock()

	latestState.Behavior = determineBehaviour(d)
	latestState.Floor = lastFloor
	latestState.Direction = motorDirectionToString(*d)
	latestState.LocalRequests = elevatorOrders
}

func turnOffHallLights(orders ...Order) {
	// Turn off the button lamp at the current floor
	for _, order := range orders {
		if order.OrderType == hall { // Hall button
			if order.Direction == up { // Hall up
				elevio.SetButtonLamp(elevio.BT_HallUp, order.Floor, false)
			} else { // Hall down
				elevio.SetButtonLamp(elevio.BT_HallDown, order.Floor, false)
			}
		}
	}

}

func turnOffCabLights(orders ...Order) { // Turn off the lights for the current order
	for _, order := range orders {
		if order.OrderType == cab {
			elevio.SetButtonLamp(elevio.BT_Cab, order.Floor, false)
		}
	}

}

func turnOffAllLights(current_order Order) {
	for f := 0; f < numFloors; f++ {
		for b := ButtonType(0); b < 3; b++ {
			elevio.SetButtonLamp(elevio.ButtonType(b), f, false)
		}
	}
}

func trackPosition(drv_floors2 chan int, drv_DirectionChange chan elevio.MotorDirection, d *elevio.MotorDirection) { // Track the position of the elevator
	for {
		select {
		case a := <-drv_floors2:
			lockMutexes(&mutex_posArray, &mutex_d)
			// Even indices are floors, odd indices are in-between floors
			// Get the current floor

			currentFloor := 0
			for i := 0; i < 2*numFloors-1; i++ {
				if posArray[i] {
					currentFloor = i
				}
			}

			if a == -1 {

				if *d == elevio.MD_Up {
					posArray[currentFloor] = false
					posArray[currentFloor+1] = true
				}
				if *d == elevio.MD_Down {
					posArray[currentFloor] = false
					posArray[currentFloor-1] = true
				}
			} else {

				posArray[currentFloor] = false
				posArray[a*2] = true

				// Set the floor indicator
				elevio.SetFloorIndicator(a)

			}

			unlockMutexes(&mutex_posArray, &mutex_d)
		case new_dir := <-drv_DirectionChange:
			lockMutexes(&mutex_posArray, &mutex_d)

			currentFloor := 0
			for i := 0; i < 2*numFloors-1; i++ {
				if posArray[i] {
					currentFloor = i
				}
			}

			switch {
			case new_dir == elevio.MD_Up:
				posArray[currentFloor] = false
				posArray[currentFloor+1] = true
			case new_dir == elevio.MD_Down:
				posArray[currentFloor] = false
				posArray[currentFloor-1] = true
			case new_dir == elevio.MD_Stop:
				// If the direction is alreadt MD_Stop we don't have to alter positionArray
			}

			unlockMutexes(&mutex_posArray, &mutex_d)
		}

	}
}

// Helper function to convert direction to string
func directionToString(direction OrderDirection) string {
	if direction == up {
		return "Up"
	}
	return "Down"
}

// Helper function to convert orderType to string
func orderTypeToString(orderType OrderType) string {
	if orderType == hall {
		return "Hall"
	}
	return "Cab"
}

func printOrders(orders []Order) {
	// Iterate through each order and print its details
	for i, order := range orders {
		fmt.Printf("Order #%d: Floor %d, Direction %s, OrderType %s\n",
			i+1, order.Floor, directionToString(order.Direction), orderTypeToString(order.OrderType))
	}
}

func printElevatorOrders(elevatorOrders []Order) {
	printOrders(elevatorOrders)
}

func reverseDirection(d *elevio.MotorDirection) { // Reverse the direction of the elevator
	switch {
	case *d == elevio.MD_Down:
		*d = elevio.MD_Up
	case *d == elevio.MD_Up:
		*d = elevio.MD_Down
	case *d == elevio.MD_Stop:
	}
}

// This function is only used internally in the sorting functions
func updatePosArray(dir elevio.MotorDirection, posArray *[2*numFloors - 1]bool) {
	// Reset all values in the array to false
	for i := range posArray {
		(posArray[i]) = false
	}

	switch {
	case dir == elevio.MD_Down:
		posArray[2*numFloors-2] = true
	case dir == elevio.MD_Up:
		posArray[0] = true
	default:
		panic("Error: MotorDirection MD_Stop passed into updatePosArray function")
	}
}

func extractPos() float32 { // Extract the current position of the elevator
	currentFloor := float32(0)
	for i := 0; i < 2*numFloors-1; i++ {
		if posArray[i] {
			currentFloor = float32(i) / 2
		}
	}
	return currentFloor
}

func addOrder(floor int, direction OrderDirection, typeOrder OrderType) { // Add an order to the elevatorOrders
	exists := false

	if typeOrder == cab {
		for _, order := range elevatorOrders {
			if order.Floor == floor && order.OrderType == cab {
				exists = true
			}
		}
	} else if typeOrder == hall {
		for _, order := range elevatorOrders {
			if order.Floor == floor && order.Direction == direction && order.OrderType == hall {
				exists = true
			}
		}
	}

	if !exists {
		elevatorOrders = append(elevatorOrders, Order{Floor: floor, Direction: direction, OrderType: typeOrder})
	}
}

// This function deletes relevant orders at the same floor as the current order,
// It takes into account if there are multiple orders to the same floor
// Since elevatorOrders is sorted, we can just delete from left to right until there are no orders with the same floor left
func PopOrders() {
	//fmt.Printf("Before deleting orders from elevatorOrders: %v\n", elevatorOrders)
	if len(elevatorOrders) != 0 {
		floor_to_pop := elevatorOrders[0].Floor

		// Figure out how many elements to delete
		ndelete := 0
		for _, order := range elevatorOrders {
			if order.Floor == floor_to_pop {
				ndelete += 1
			} else {
				break
			}
		}

		// Now that we've calculated the number of elements to delete, update elevatorOrders
		elevatorOrders = elevatorOrders[ndelete:]
	}
	//fmt.Printf("After deleting orders from elevatorOrders: %v\n", elevatorOrders)
}

func changeDirBasedOnCurrentOrder(d *elevio.MotorDirection, current_order Order, current_floor float32) { // Change the direction based on the current order
	switch {
	case current_floor > float32(current_order.Floor):
		*d = elevio.MD_Down
	case current_floor < float32(current_order.Floor):
		*d = elevio.MD_Up
	case current_floor == float32(current_order.Floor):
		*d = elevio.MD_Stop
	}
}

func StopBlocker(Inital_duration time.Duration) { // Block the elevator for a certain duration
	Timer := Inital_duration
	sleepDuration := 30 * time.Millisecond
outerloop:
	for {
		switch {
		case Timer <= time.Duration(0):
			elevio.SetDoorOpenLamp(false)
			break outerloop
		case Timer > time.Duration(0):
			mutex_doors.Lock()
			switch {
			case ableToCloseDoors:
				Timer = Timer - sleepDuration
			case !ableToCloseDoors:
				Timer = Inital_duration
			}
			mutex_doors.Unlock()
		}
		time.Sleep(sleepDuration)
	}
}

func relay(source chan int, consumers ...chan int) {
	for {
		value := <-source
		for _, consumer := range consumers {
			consumer <- value // Send to each consumer
		}
	}
}

// This function will attend to the current order, it
func attendToSpecificOrder(d *elevio.MotorDirection, consumer2drv_floors chan int, drv_newOrder chan Order, drv_DirectionChange chan elevio.MotorDirection) {
	current_order := Order{0, -1, 0}
	for {
		select {
		case a := <-consumer2drv_floors: // Triggers when we arrive at a new floor
			lockMutexes(&mutex_d, &mutex_elevatorOrders, &mutex_posArray)
			if a == current_order.Floor { // Check if our new floor is equal to the floor of the order
				// Set direction to stop and delete relevant orders from elevatorOrders

				*d = elevio.MD_Stop
				elevio.SetMotorDirection(*d)

				// Clear the cab lights for this order, (the removal of hallOrders is sent through the MasterRoutine and back to all single elevators)
				turnOffCabLights(current_order)
				// In case we lose connection to the masterRoutine
				turnOffHallLights(current_order)

				PopOrders()

				elevio.SetDoorOpenLamp(true)
				StopBlocker(3000 * time.Millisecond)
				elevio.SetDoorOpenLamp(false)

				// After deleting the relevant orders at our floor => find, if any, the next currentOrder
				if len(elevatorOrders) != 0 {
					current_order = elevatorOrders[0]
					prev_direction := *d
					changeDirBasedOnCurrentOrder(d, current_order, float32(a))
					new_direction := *d

					elevio.SetMotorDirection(*d)

					// Communicate with trackPosition if our direction was altered
					unlockMutexes(&mutex_d, &mutex_posArray)
					if prev_direction != new_direction {
						drv_DirectionChange <- new_direction
					}
					lockMutexes(&mutex_d, &mutex_posArray)
				} else {
					turnOffAllLights(current_order)
				}
			}
			unlockMutexes(&mutex_d, &mutex_elevatorOrders, &mutex_posArray)
		case a := <-drv_newOrder: // If we get a new order => update current order and see if we need to redirect our elevator
			lockMutexes(&mutex_d, &mutex_elevatorOrders, &mutex_posArray)

			current_order = a
			current_position := extractPos()
			switch {
			// Case 1: HandleOrders sent a new Order and it is at the same floor
			case *d == elevio.MD_Stop && current_position == float32(current_order.Floor):
				turnOffCabLights(current_order) // Clear the cab lights for this order
				turnOffHallLights(current_order)

				elevio.SetDoorOpenLamp(true)
				StopBlocker(3000 * time.Millisecond)
				elevio.SetDoorOpenLamp(false)

				PopOrders()

				// After deleting the relevant orders at our floor => find, if any, find the next currentOrder
				if len(elevatorOrders) != 0 {
					current_order = elevatorOrders[0]
					prev_direction := *d
					changeDirBasedOnCurrentOrder(d, current_order, float32(current_order.Floor))
					new_direction := *d

					elevio.SetMotorDirection(*d)

					// Communicate with trackPosition if our direction was altered
					unlockMutexes(&mutex_d, &mutex_posArray)
					if prev_direction != new_direction {
						drv_DirectionChange <- new_direction
					}
					lockMutexes(&mutex_d, &mutex_posArray)
				} else {
					turnOffAllLights(current_order)
				}

				// Case 2: HandleOrders sent a new Order and it is at a different floor
			case current_position != float32(current_order.Floor):
				current_position := extractPos()

				prev_direction := *d
				changeDirBasedOnCurrentOrder(d, current_order, current_position)
				new_direction := *d

				elevio.SetDoorOpenLamp(false) // Just in case

				elevio.SetMotorDirection(*d)

				// Communicate with trackPosition if our direction was altered
				unlockMutexes(&mutex_d, &mutex_posArray)
				if prev_direction != new_direction {
					drv_DirectionChange <- new_direction
				}
				lockMutexes(&mutex_d, &mutex_posArray)
			}

			unlockMutexes(&mutex_d, &mutex_elevatorOrders, &mutex_posArray)
		}
	}
}
