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

func MasterRoutine(hallBtnRx chan elevio.ButtonEvent, stateRx chan StateMsg, hallOrderTx chan HallOrderMsg) {

	// Define an array of elevator states for continously monitoring the elevators
	// It will be updated whenever we receive a new state from the slaves
	var allStates [numElev]ElevState
	uninitializedOrderArray := []Order{
		{
			floor:     0,
			direction: up,   // Replace with your OrderDirection constant (e.g., Up or Down)
			orderType: hall, // Replace with your OrderType constant (e.g., Hall or Cab)
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

			// Serialize hallOrdersAndId using go
			/*
				var b bytes.Buffer
				enc := gob.NewEncoder(&b)
				if err := enc.Encode(HallOrderMsg); err != nil {
					fmt.Println("Error encoding HallOrderMsg:", err)
					return
				}
				// Send the serialized hallOrder over the channel
			*/

			hallOrderTx <- HallOrderMessage

			fmt.Printf("CodeExcecutionEnd - hallBtnRx in MasterRoutine\n")
		case a := <-stateRx:
			// Update our list of allStates with the new state

			allStates[a.Id] = a.State
		}
	}
}

func PrimaryRoutine() {

}

func InitializeNetwork(role string, id int, hallOrderRx chan HallOrderMsg, hallBtnTx chan elevio.ButtonEvent, singleStateTx chan StateMsg) {
	// NETWORK CHANNELS (For all)

	// Receive orders from master
	go bcast.Receiver(HallOrder_PORT, hallOrderRx)

	// Transmit raw hall buttons
	go bcast.Transmitter(HallOrderRawBTN_PORT, hallBtnTx)

	// Transmit elevator states
	go bcast.Transmitter(SingleElevatorState_PORT, singleStateTx)

	// Role-specific logic
	switch role {
	case "Master":
		hallOrderTx := make(chan HallOrderMsg)
		go bcast.Transmitter(HallOrder_PORT, hallOrderTx)

		hallBtnRx := make(chan elevio.ButtonEvent)
		go bcast.Receiver(HallOrderRawBTN_PORT, hallBtnRx)

		singleStateRx := make(chan StateMsg)
		go bcast.Receiver(SingleElevatorState_PORT, singleStateRx)

		go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx)
	case "Primary":
		// Placeholder for Primary-specific logic
	}
	fmt.Println("Network initialized.")
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
