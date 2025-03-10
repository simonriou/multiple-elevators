package main

import (
	"Driver-go/elevio"
)

func MasterRoutine(hallBtnTx chan elevio.ButtonEvent, hallBtnRx chan elevio.ButtonEvent, stateRx chan ElevState, orderTx chan Order) {

	// Define datatypes for master
	// Define a slice that represents all the states of the elevators

	// Define datatypes for keeping track of which elevator is active and not

	for {
		select {
		// Listen for updates on the ElevStateMessage channel

		// Listen for updates on the hallBtnRx

		}
	}
}

func PrimaryRoutine() {

}
