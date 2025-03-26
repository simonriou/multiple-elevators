package main

import (
	"Driver-go/elevio"
	"math"
	"time"
	"fmt"
)

func findHighestOrders(elevatorOrders []Order) []Order {
	// Set the initial highest Floor to a very low value
	highestFloor := -1
	var highestOrders []Order

	// Loop through each order to find the highest Floor
	for _, order := range elevatorOrders {
		if order.Floor > highestFloor {
			// If we find a higher Floor, reset the slice and add the current order
			highestFloor = order.Floor
			highestOrders = []Order{order}
		} else if order.Floor == highestFloor {
			// If the Floor matches the current highest, add it to the slice
			highestOrders = append(highestOrders, order)
		}
	}

	return highestOrders
}

func findLowestOrders(elevatorOrders []Order) []Order {
	// Set the initial lowest Floor to a very high value
	lowestFloor := 1000000
	var lowestOrders []Order

	// Loop through each order to find the lowest Floor
	for _, order := range elevatorOrders {
		if order.Floor < lowestFloor {
			// If we find a lower Floor, reset the slice and add the current order
			lowestFloor = order.Floor
			lowestOrders = []Order{order}
		} else if order.Floor == lowestFloor {
			// If the Floor matches the current lowest, add it to the slice
			lowestOrders = append(lowestOrders, order)
		}
	}

	return lowestOrders
}

func orderInContainer(order_slice []Order, order_ Order) bool {
	for _, v := range order_slice {
		if v == order_ {
			return true
		}
	}
	return false
}

// This function will attend to the current order, it
func attendToSpecificOrder(d *elevio.MotorDirection, consumer2drv_floors chan int, drv_newOrder chan Order, drv_DirectionChange chan elevio.MotorDirection,
	singleStateTx chan StateMsg, id int) {
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
				updateState(d, current_order.Floor, elevatorOrders, &latestState)
				singleStateTx <- StateMsg{id, latestState}

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
					turnOffAllLights()
				}
			}
			unlockMutexes(&mutex_d, &mutex_elevatorOrders, &mutex_posArray)
		case a := <-drv_newOrder: // If we get a new order => update current order and see if we need to redirect our elevator
			fmt.Printf("Received order in attendToSpecificOrder\n")
			lockMutexes(&mutex_d, &mutex_elevatorOrders, &mutex_posArray)
			fmt.Printf("Made it past the mutex locks in attendtospecific\n")

			current_order = a
			current_position := extractPos()
			switch {
			// Case 1: HandleOrders sent a new Order and it is at the same floor
			case *d == elevio.MD_Stop && current_position == float32(current_order.Floor):
				turnOffCabLights(current_order) // Clear the cab lights for this order
				turnOffHallLights(current_order)

				PopOrders()
				updateState(d, current_order.Floor, elevatorOrders, &latestState)
				singleStateTx <- StateMsg{id, latestState}

				elevio.SetDoorOpenLamp(true)
				StopBlocker(3000 * time.Millisecond)
				elevio.SetDoorOpenLamp(false)

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
					turnOffAllLights()
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



func sortOrdersInDirection(elevatorOrders []Order, d elevio.MotorDirection, posArray [2*numFloors - 1]bool) ([]Order, []Order, elevio.MotorDirection) {

	highestOrders := findHighestOrders(elevatorOrders)
	lowestOrders := findLowestOrders(elevatorOrders)

	//Calculating the current Floor as a decimal so that its compareable to
	currentFloor := float32(0)
	for i := 0; i < 2*numFloors-1; i++ {
		if posArray[i] {
			currentFloor = float32(i) / 2
		}
	}

	// Section_START handle d==MD_Stop
	if d == elevio.MD_Stop {
		// Find Direction based on cab order
		num_cabOrdersAbove := 0
		num_cabOrdersBelow := 0
		closest := Order{100000, 1, 1}

		for _, order := range elevatorOrders {
			floor_order := float32(order.Floor)
			if math.Abs(float64(currentFloor)-float64(order.Floor)) < float64(closest.Floor) {
				closest = order
			}
			switch {
			case floor_order > currentFloor:
				num_cabOrdersAbove += 1
			case floor_order < currentFloor:
				num_cabOrdersBelow += 1
			}
		}

		switch {
		case num_cabOrdersAbove > num_cabOrdersBelow:
			d = elevio.MD_Up
		case num_cabOrdersAbove < num_cabOrdersBelow:
			d = elevio.MD_Down
		case num_cabOrdersAbove == num_cabOrdersBelow:
			if float32(closest.Floor) > float32(currentFloor) {
				d = elevio.MD_Up
			} else {
				d = elevio.MD_Down
			}
		}
	}

	// Section_END handle d==MD_Stop

	//Based current Direction => find all the equiDirectional orders plus potential extremities
	//Store the relevant orders in relevantOrders and the rest in irrelevantOrders
	relevantOrders := []Order{}
	irrelevantOrders := []Order{}

	for _, order := range elevatorOrders {
		inHighest := orderInContainer(highestOrders, order)
		inLowest := orderInContainer(lowestOrders, order)

		//We define a variable for measuring the distance between current_pos and order.
		//Positive -> The order is above us
		//Zero     -> The order is at the same Floor
		//Negative -> The order is below us
		distOrderToCurrent := float32(order.Floor) - currentFloor
		switch {
		case (d == elevio.MD_Up) && (distOrderToCurrent >= 0.0): //If we're going up and the order is above/same
			switch {
			case inHighest:
				relevantOrders = append(relevantOrders, order)
			case order.Direction == up || order.OrderType == cab:
				relevantOrders = append(relevantOrders, order)
			case order.Direction == down:
				irrelevantOrders = append(irrelevantOrders, order)
			}
		case (d == elevio.MD_Up) && (distOrderToCurrent < 0.0): //If we're going up and the order is below/same
			irrelevantOrders = append(irrelevantOrders, order)

		case (d == elevio.MD_Down) && (distOrderToCurrent <= 0.0): //If we're going down and the order is below/same
			//If order is down or cab
			switch {
			case inLowest:
				relevantOrders = append(relevantOrders, order)
			case order.Direction == down || order.OrderType == cab:
				relevantOrders = append(relevantOrders, order)
			case order.Direction == up:
				irrelevantOrders = append(irrelevantOrders, order)
			}
		case (d == elevio.MD_Down) && (distOrderToCurrent > 0.0): //If we're going down and the order is up/same
			irrelevantOrders = append(irrelevantOrders, order)
		}

	}

	//Now that we've seperated the relevant and irrellevant orders from each other, we sort the relevant part
	//If the current Direction is up, we sort them in ascending order
	if d == elevio.MD_Up {
		n := len(relevantOrders)
		for i := 0; i < n-1; i++ {
			// Last i elements are already sorted
			for j := 0; j < n-i-1; j++ {
				if relevantOrders[j].Floor > relevantOrders[j+1].Floor {
					// Swap arr[j] and arr[j+1]
					relevantOrders[j], relevantOrders[j+1] = relevantOrders[j+1], relevantOrders[j]
				}
			}
		}
	}

	//If the current Direction is down, we sort them in descending order
	if d == elevio.MD_Down {
		//Perform bubblesort in descending order
		n := len(relevantOrders)
		for i := 0; i < n-1; i++ {
			// Last i elements are already sorted
			for j := 0; j < n-i-1; j++ {
				if relevantOrders[j].Floor < relevantOrders[j+1].Floor {
					// Swap arr[j] and arr[j+1]
					relevantOrders[j], relevantOrders[j+1] = relevantOrders[j+1], relevantOrders[j]
				}
			}
		}
	}

	return relevantOrders, irrelevantOrders, d
}

func sortAllOrders(elevatorOrders *[]Order, d elevio.MotorDirection, posArray [2*numFloors - 1]bool) {
	if len(*elevatorOrders) == 0 || len(*elevatorOrders) == 1 {
		return
	}

	// Handle that rare case where the motorDirection is MD_Stop and we have multiple orders

	// fmt.Printf("Made it past the inital checks in sortAllOrders\n")

	// Creating the datatypes specfic to our function
	copy_posArray := posArray
	relevantOrders := []Order{}
	_ = relevantOrders
	irrelevantOrders := []Order{}
	_ = irrelevantOrders

	// Start - first section
	firstSection := []Order{}
	_ = firstSection

	irrelevantOrders = *elevatorOrders
	relevantOrders, irrelevantOrders, d = sortOrdersInDirection(irrelevantOrders, d, copy_posArray)
	firstSection = relevantOrders

	if len(irrelevantOrders) == 0 {
		*elevatorOrders = firstSection
		return
	}
	// End - First Section

	// Start - Second section
	secondSection := []Order{}
	_ = secondSection
	reverseDirection(&d)
	updatePosArray(d, &copy_posArray)

	relevantOrders, irrelevantOrders, d = sortOrdersInDirection(irrelevantOrders, d, copy_posArray)
	secondSection = relevantOrders

	if len(irrelevantOrders) == 0 {
		*elevatorOrders = append(firstSection, secondSection...)
		return
	}
	// End - Second section

	// Start - Third section
	thirdSection := []Order{}
	_ = thirdSection
	reverseDirection(&d)
	updatePosArray(d, &copy_posArray)
	relevantOrders, _, _ = sortOrdersInDirection(irrelevantOrders, d, copy_posArray)
	thirdSection = relevantOrders
	// End - Third section

	*elevatorOrders = append(firstSection, secondSection...)
	*elevatorOrders = append(*elevatorOrders, thirdSection...)
}
