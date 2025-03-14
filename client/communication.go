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

func MasterRoutine(hallBtnRx chan elevio.ButtonEvent, singleStateRx chan StateMsg, hallOrderTx chan HallOrderMsg, AllStatesTx chan [numElev]ElevState) {

	go bcast.Receiver(HallOrderRawBTN_PORT, hallBtnRx)
	go bcast.Receiver(SingleElevatorState_PORT, singleStateRx)
	go bcast.Transmitter(HallOrder_PORT, hallOrderTx)
	go bcast.Transmitter(AllStates_PORT, AllStatesTx)

	// Define an array of elevator states for continously monitoring the elevators
	// It will be updated whenever we receive a new state from the slaves
	var allStates [numElev]ElevState
	uninitializedOrderArray := []Order{
		{
			Floor:     0,
			Direction: up,   // Replace with your OrderDirection constant (e.g., Up or Down)
			OrderType: hall, // Replace with your OrderType constant (e.g., Hall or Cab)
		},
	}
	uninitialized_ElevState := ElevState{
		Behavior:      "Uninitialized",
		Floor:         -2,
		Direction:     "Uninitialized",
		LocalRequests: uninitializedOrderArray,
	}

	for i := range allStates {
		allStates[i] = uninitialized_ElevState
	}

	for {
		select {
		case a := <-hallBtnRx:
			fmt.Printf("CodeExcecutionStart - hallBtnRx in MasterRoutine\n")
			// Calculate the cost of assigning the order to each elevator
			orderCosts := [numElev]float64{}

			PrintButtonEvent(a)
			for i, state := range allStates { // Assuming the elevators are sorted by ID inside of allStates
				if state.Behavior != "Uninitialized" {
					cost := calculateCost(state, btnPressToOrder(a))
					fmt.Printf("The cost of assigning the hall order to elevator: %d, with the corresponding state: %v, is: %f\n", i, state, cost)
					orderCosts[i] = cost
				}
			}

			// Find the elevator with the lowest cost
			bestElevator := 0 // Id of the best elevator
			for i, cost := range orderCosts {
				if cost < orderCosts[bestElevator] {
					bestElevator = i
				}
			}

			HallOrderMessage := HallOrderMsg{bestElevator, btnPressToOrder(a)}
			fmt.Printf("HallOrderMsg sent: %v\n", HallOrderMessage)

			hallOrderTx <- HallOrderMessage

			fmt.Printf("CodeExcecutionEnd - hallBtnRx in MasterRoutine\n")
		case a := <-singleStateRx:
			// Update our list of allStates with the new state

			allStates[a.Id] = a.State
		}
	}
}

func PrimaryRoutine() {

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
	cost := math.Abs(float64(order.Floor - elevator.Floor))

	// If elevator is idle, prioritize it
	if elevator.Behavior == "idle" {
		cost *= 0.5 // Favor idle elevators
	}

	// If moving in the same direction and order is on the way, lower cost
	if (elevator.Direction == "up" && order.Direction == 1 && order.Floor >= elevator.Floor) ||
		(elevator.Direction == "down" && order.Direction == -1 && order.Floor <= elevator.Floor) {
		cost *= 0.8 // Favor elevators already moving toward the order
	}

	// If moving in the opposite direction, penalize cost
	if (elevator.Direction == "up" && order.Direction == -1) ||
		(elevator.Direction == "down" && order.Direction == 1) {
		cost *= 1.5 // Penalize opposite direction
	}

	return cost
}
