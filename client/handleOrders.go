package main

import (
	"Driver-go/elevio"
	"math"
)

type ButtonType int

const (
	BT_HallUp   ButtonType = 0
	BT_HallDown            = 1
	BT_Cab                 = 2
)

type ButtonEvent struct {
	Floor  int
	Button ButtonType
}

// Region: Data types for the orders

type OrderDirection int

const (
	up   OrderDirection = 1
	down OrderDirection = -1
)

type OrderType int

const (
	hall OrderType = 0
	cab  OrderType = 1
)

type Order struct {
	Floor     int
	Direction OrderDirection // 1 for up, -1 for down
	OrderType OrderType      // 0 for hall, 1 for cab
}

// end Region: Data types for the orders

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
	irrelevantOrders := []Order{}

	// Start - first section
	firstSection := []Order{}

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
	reverseDirection(&d)
	updatePosArray(d, &copy_posArray)
	relevantOrders, _, _ = sortOrdersInDirection(irrelevantOrders, d, copy_posArray)
	thirdSection = relevantOrders
	// End - Third section

	*elevatorOrders = append(firstSection, secondSection...)
	*elevatorOrders = append(*elevatorOrders, thirdSection...)
}
