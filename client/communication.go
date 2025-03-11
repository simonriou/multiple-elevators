package main

import (
	"Driver-go/elevio"
	"math"
	"Network-go/network/bcast"
	"Network-go/network/peers"
	"fmt"
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

	// Define an array of elevator states for continously monitoring the elevators
	// It will be updated whenever we receive a new state from the slaves
	var allStates [numElev]ElevState
	Uninitialized_ElevState := ElevState{
		Behavior:  "Uninitialized",
		Floor:     -2,
		Direction: "Uninitialized",
		Orders:    []Order{},
	}

	for i := range allStates {
		allStates[i] = Uninitialized_ElevState
	}

	// Make functionality for peer-updates
	peerUpdateCh := make(chan peers.PeerUpdate)     // Updates from peers
	peerTxEnable := make(chan bool)                 // Enables/disables the transmitter
	go peers.Transmitter(PeerChannel, role, peerTxEnable) // Broadcast role
	go peers.Receiver(PeerChannel, peerUpdateCh)          // Listen for updates

	for {
		select {
			case a := <-hallBtnRx:	
				// Calculate the cost of assigning the order to each elevator
				orderCosts := [numElev]float64{}

				PrintButtonEvent(a)
				for i, state := range allStates { // Assuming the elevators are sorted by ID inside of allStates
					if state != Uninitialized_ElevState {
						cost := calculateCost(state, btnPressToOrder(a))
						fmt.Printf("The cost of assigning the hall order to elevator: %d, is: %f\n", i, cost)
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

				// Send the order to the best elevator
				hallOrderTx <- HallOrderAndId{btnPressToOrder(a), bestElevator}

			case a := <-stateRx:
				// Update our list of allStates with the new state

				allStates[a.Id] = a.State
				
			case p := <-peerUpdateCh:
				fmt.Printf("Peer update:\n")
				fmt.Printf("  Peers:    %q\n", p.Peers)
				fmt.Printf("  New:      %q\n", p.New)
				fmt.Printf("  Lost:     %q\n", p.Lost)
		}
	}
}

func PrimaryRoutine() {

}



func InitializeNetwork(role string, id int, hallOrderRx chan Order, hallBtnTx chan elevio.ButtonEvent, singleStateTx chan StateMsg) {
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

		hallOrderTx := make(chan HallOrderAndId)
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
	case BT_HallUp:
		buttonType = "Hall Up"
	case BT_HallDown:
		buttonType = "Hall Down"
	case BT_Cab:
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
