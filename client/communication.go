package main

import (
	"Driver-go/elevio"
	"Network-go/network/bcast"
	"fmt"
	"math"
)

type HRAInput struct {
	HallRequests []Order
	States       map[string]ElevState
}

type HallOrderMsg struct {
	Id        int
	HallOrder Order
}

func extractHallOrders(orders []Order) []Order {
	var hallOrders []Order
	for _, order := range orders {
		if order.OrderType == hall { // Check if it's a HallOrder
			hallOrders = append(hallOrders, order) // Add to the hallOrders slice
		}
	}
	return hallOrders
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Function to find elements in oldStateOrders that are not in newStateOrders and vice versa
func findUniqueOrders(oldOrders, newOrders []Order) []Order {
	// Use maps to track orders
	oldOrdersMap := make(map[Order]bool)
	newOrdersMap := make(map[Order]bool)
	var uniqueOrders []Order

	// Add all old orders to oldOrdersMap
	for _, order := range oldOrders {
		oldOrdersMap[order] = true
	}

	// Add all new orders to newOrdersMap
	for _, order := range newOrders {
		newOrdersMap[order] = true
	}

	// Find orders in oldOrdersMap that are not in newOrdersMap
	for order := range oldOrdersMap {
		if !newOrdersMap[order] {
			uniqueOrders = append(uniqueOrders, order)
		}
	}

	// Find orders in newOrdersMap that are not in oldOrdersMap
	for order := range newOrdersMap {
		if !oldOrdersMap[order] {
			uniqueOrders = append(uniqueOrders, order)
		}
	}

	return uniqueOrders
}

func MasterRoutine(hallBtnRx chan elevio.ButtonEvent, singleStateRx chan StateMsg, hallOrderTx chan HallOrderMsg,
	backupStatesTx chan [numElev]ElevState, newStatesRx chan [numElev]ElevState,
	hallOrderCompletedTx chan []Order) {

	go bcast.Receiver(HallOrderRawBTN_PORT, hallBtnRx)
	go bcast.Receiver(SingleElevatorState_PORT, singleStateRx)
	go bcast.Transmitter(HallOrder_PORT, hallOrderTx)
	go bcast.Transmitter(AllStates_PORT, backupStatesTx)
	go bcast.Receiver(BackupStates_PORT, newStatesRx)
	go bcast.Transmitter(HallOrderCompleted_PORT, hallOrderCompletedTx)

	// Define an array of elevator states for continously monitoring the elevators
	// It will be updated whenever we receive a new state from the slaves
	var allStates = <-newStatesRx

	for {
		select {
		case a := <-hallBtnRx:

			// Retrieves the information on the working elevators
			var workingElevNb = len(activeElevators)
			workingElevs := make([]ElevState, workingElevNb)
			// Remember which index coresponds to which elevator id
			// This is important for sending the hall order to the correct elevator
			indexMapping := []int{} // Contains the id of the working elevators in the order they are in workingElevs
			for i, id := range activeElevators {
				workingElevs[i] = allStates[id]
				indexMapping = append(indexMapping, id)
			}

			// Calculate the cost of assigning the order to each elevator
			orderCosts := make([]float64, workingElevNb)

			PrintButtonEvent(a)
			for i, state := range workingElevs {
				if state.Behavior != "Uninitialized" {
					cost := calculateCost(state, btnPressToOrder(a))
					fmt.Printf("The cost of assigning the hall order to elevator: %d, with the corresponding state: %v, is: %f\n", i, state, cost)
					orderCosts[i] = cost
				}
			}

			// Find the elevator with the lowest cost
			bestElevator := 0 // Id of the best elevator (relative to workingElevs)
			for i, cost := range orderCosts {
				if cost < orderCosts[bestElevator] {
					bestElevator = i
				}
			}

			// Retrieve the id of the best elevator (relative to allStates)
			bestElevator = indexMapping[bestElevator]

			HallOrderMessage := HallOrderMsg{bestElevator, btnPressToOrder(a)}

			// Send the order to a slave
			hallOrderTx <- HallOrderMessage

		case a := <-singleStateRx: // A state update on singleStateRx

			// Compare the old and new state and send a message on orderCompleted so that the order lights get taken care of
			//        Assume that we dont delete and add hallOrders at the same time
			oldStateOrders := allStates[a.Id].LocalRequests
			newStateOrders := a.State.LocalRequests

			oldHallOrders := extractHallOrders(oldStateOrders)
			newHallOrders := extractHallOrders(newStateOrders)
			length_old := len(oldHallOrders)
			length_new := len(newHallOrders)
			if length_new < length_old {
				removed_hallOrders := findUniqueOrders(oldHallOrders, newHallOrders)
				hallOrderCompletedTx <- removed_hallOrders
			}

			findUniqueOrders(oldHallOrders, newHallOrders)

			_ = oldHallOrders
			_ = newHallOrders

			// Update our list of allStates with the new state and send new states list to the primary backup
			allStates[a.Id] = a.State
			backupStatesTx <- allStates
		}
	}
}

func PrimaryBackupRoutine(backupStatesRx chan [numElev]ElevState) {

	// To-Do: update the global backupStates
	go bcast.Receiver(AllStates_PORT, backupStatesRx) // Used to receive the states from the master

	for {
		select {
		case a := <-backupStatesRx:
			// Update the global backupStates
			mutex_backup.Lock()
			backupStates = a
			mutex_backup.Unlock()
		}
	}

}

func PrintButtonEvent(event elevio.ButtonEvent) {
	var buttonType string
	switch event.Button {
	case elevio.BT_HallUp:
		buttonType = "Hall Up"
	case elevio.BT_HallDown:
		buttonType = "Hall Down"
	case elevio.BT_Cab:
		buttonType = "Cab"
	default:
		buttonType = "Unknown"
	}
	fmt.Printf("Button Event - Floor: %d, Button: %s\n", event.Floor, buttonType)
}

// Cost function for assigning an order to an elevator.
func calculateCost(elevator ElevState, order Order) float64 {
	// Base cost is the absolute distance from the elevator to the order
	cost := 1 + math.Abs(float64(order.Floor-elevator.Floor))

	// If elevator is idle, prioritize it
	if elevator.Behavior == "idle" {
		cost *= 0.5 // Strongly favor idle elevators
	}

	if elevator.Behavior == "moving" && order.Floor == elevator.Floor {
		cost *= 2
	}

	// If moving in the same direction and order is on the way, lower cost
	if (elevator.Direction == "up" && order.Direction == 1 && order.Floor >= elevator.Floor) ||
		(elevator.Direction == "down" && order.Direction == -1 && order.Floor <= elevator.Floor) {
		cost *= 0.5 // Favor elevators already moving toward the order
	}

	// If moving in the opposite direction, penalize cost
	if (elevator.Direction == "up" && order.Direction == -1) ||
		(elevator.Direction == "down" && order.Direction == 1) {
		cost *= 1.5 // Penalize opposite direction
	}

	return cost
}

/*
// Cost function for assigning an order to an elevator.
func calculateCost(elevator ElevState, order Order) float64 {
	// Base cost is the absolute distance from the elevator to the order
	cost := math.Abs(float64(order.Floor - elevator.Floor))
	if elevator.Behavior == "Idle" {
		// If the elevator is idle, we add a small cost to the cost of the order
		return cost
	}

	return cost
}
*/
