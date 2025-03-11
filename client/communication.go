package main

import (
	"Driver-go/elevio"
)

type HRAInput struct {
	HallRequests [][2]bool            `json:"hallRequests"`
	States       map[string]ElevState `json:"states"`
}

func MasterRoutine(hallBtnTx chan elevio.ButtonEvent, hallBtnRx chan elevio.ButtonEvent, stateRx chan ElevState, orderTx chan Order) {

	// Define datatypes for master
	// Define a slice that represents all the states of the elevators

	// Define datatypes for keeping track of which elevator is active and not

	for {
		select {
		// When getting new hall button presses from the elevators
		// Add them to the global order list that will be used in the input to the HRA (HallRequests list)

		// When getting new states updates from the elevators
		// Update the global state list
		}
	}
}

func PrimaryRoutine() {

}
