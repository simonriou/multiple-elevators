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

func MasterRoutine(hallBtnRx chan elevio.ButtonEvent, singleStateRx chan StateMsg, hallOrderTx chan HallOrderMsg,
	backupStatesTx chan [numElev]ElevState, newStatesRx chan [numElev]ElevState) {

	go bcast.Receiver(HallOrderRawBTN_PORT, hallBtnRx)
	go bcast.Receiver(SingleElevatorState_PORT, singleStateRx)
	go bcast.Transmitter(HallOrder_PORT, hallOrderTx)
	go bcast.Transmitter(AllStates_PORT, backupStatesTx)

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

		case a := <-singleStateRx:
			// Update our list of allStates with the new state
			allStates[a.Id] = a.State

			// Send the new states list to the primary backup
			backupStatesTx <- allStates
		}
	}
}

func PrimaryBackupRoutine(backupStatesRx chan [numElev]ElevState, newStatesTx chan [numElev]ElevState) {

	go bcast.Receiver(BackupStates_PORT, backupStatesRx) // Used to receive the states from the master
	go bcast.Transmitter(BackupStates_PORT, newStatesTx) // Used to send the states to the NEW master

	// To-Do: update the global backupStates

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
