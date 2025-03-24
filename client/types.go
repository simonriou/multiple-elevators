// This file contains the type declarations for the client
package main

type ElevState struct { // Struct for the state of the elevator
	Behavior      string  // 'moving' or 'idle'
	Floor         int     // The floor the elevator is at
	Direction     string  // 'up', 'down' or 'stop'
	LocalRequests []Order // The requests of the elevator
}

type HRAInput struct {
	HallRequests []Order
	States       map[string]ElevState
}

type HallOrderMsg struct {
	Id        int
	HallOrder Order
}

type CabOrderMsg struct {
	Id int
	CabOrder Order
}

type StateMsg struct { // Structure used to send states to the master
	Id    int
	State ElevState
}

type ButtonType int // Enum for the button types

type ButtonEvent struct { // Struct for the button events
	Floor  int
	Button ButtonType
}

type OrderDirection int // Enum for the order directions

type OrderType int // Enum for the order types

type Order struct { // Struct for the orders
	Floor     int
	Direction OrderDirection // 1 for up, -1 for down
	OrderType OrderType      // 0 for hall, 1 for cab
}
