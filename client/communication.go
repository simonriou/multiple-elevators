package main

import (
	"Driver-go/elevio"
	"math"
)

type HRAInput struct {
	HallRequests []Order
	States       map[string]ElevState
}

type HallOrderAndId struct {
	hallOrder Order
	id        int
}

func MasterRoutine(hallBtnRx chan elevio.ButtonEvent, stateRx chan StateMsg, hallOrderTx chan HallOrderAndId) {

	// Define a slice of States that will be updated whenever we receive a new state from the slaves
	allStates := make([]ElevState, numElev)

	for {
		select {
		case a := <-hallBtnRx:
			// Calculate the cost of assigning the order to each elevator
			// Assign the order to the elevator with the lowest cost
			// Send the order to the elevator
			orderCosts := [numElev]float64{}
			for i, state := range allStates { // Assuming the elevators are sorted by ID inside of allStates
				orderCosts[i] = calculateCost(state, btnPressToOrder(a))
			}

			// Find the elevator with the lowest cost
			bestElevator := 0 // Id of the best elevator
			for i, cost := range orderCosts {
				if cost < orderCosts[bestElevator] {
					bestElevator = i
				}
			}

			// Send the order to the best elevator
			hallOrderTx <- HallOrderAndId{btnPressToOrder(a), bestElevator}

		case a := <-stateRx:
			// Update our list of allStates with the new state

			allStates[a.Id] = a.State
		}
	}
}

func PrimaryRoutine() {

}

// Cost function for assigning an order to an elevator.
func calculateCost(elevator ElevState, order Order) float64 {
	// Base cost is the absolute distance from the elevator to the order
	cost := math.Abs(float64(order.floor - elevator.Floor))

	// If elevator is idle, prioritize it
	if elevator.Behavior == "idle" {
		cost *= 0.5 // Favor idle elevators
	}

	// If moving in the same direction and order is on the way, lower cost
	if (elevator.Direction == "up" && order.direction == 1 && order.floor >= elevator.Floor) ||
		(elevator.Direction == "down" && order.direction == -1 && order.floor <= elevator.Floor) {
		cost *= 0.8 // Favor elevators already moving toward the order
	}

	// If moving in the opposite direction, penalize cost
	if (elevator.Direction == "up" && order.direction == -1) ||
		(elevator.Direction == "down" && order.direction == 1) {
		cost *= 1.5 // Penalize opposite direction
	}

	return cost
}
