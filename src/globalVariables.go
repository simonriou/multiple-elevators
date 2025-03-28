// This file contains the declaration of all global variables
package main

import (
	"Driver-go/elevio"
	"sync"
	"time"
)

const numFloors = 4 // Number of floors
const numElev = 3   // Number of elevators

const ( // Ports
	HallOrder_PORT           = 16120 + iota // Send hall orders (slave <-> master)
	HallOrderRawBTN_PORT                    // Send hall orders (raw button presses)
	SingleElevatorState_PORT                // Send the state of a single elevator (master <-> slave)
	AllStates_PORT                          // Send the states of all elevators (master <-> primary backup)
	PeerChannel_PORT                        // Peer channel update port (all)
	BackupStates_PORT                       // Backup states port (master <-> primary backup)
	HallOrderCompleted_PORT                 // Hall order completed port (slave <-> master)
	ActiveElevators_PORT                    // Active elevators port (all)
	RetrieveCabOrders_PORT                  // Retrieve cab orders port (slave <-> master)
	AskForCabOrders_PORT                    // Ask for cab orders port (master <-> slave)
	MissingElev_PORT                        // Missing elevator port (all)
	AskForMissingInfo_PORT                  // Ask for missing info port (all)
)

const (
	BT_HallUp   ButtonType = 0
	BT_HallDown ButtonType = 1
	BT_Cab      ButtonType = 2
)

const (
	up   OrderDirection = 1
	down OrderDirection = -1
)

const (
	hall OrderType = 0
	cab  OrderType = 1
)

var (
	elevatorOrders       []Order // The local orders array for this elevator
	mutex_elevatorOrders sync.Mutex
)

var (
	posArray       [2*numFloors - 1]bool // The position array for this elevator
	mutex_posArray sync.Mutex
)

var (
	ableToCloseDoors bool // A boolean that tells us if we are able to close the doors
	mutex_doors      sync.Mutex
)

var (
	role string // The role of the elevator (Master, Slave or PrimaryBackup)
)

var (
	lastFloor int // The last floor the elevator was at
)

var (
	latestState ElevState // The latest state of the elevator
	mutex_state sync.Mutex
)

var ( // The active elevators
	activeElevators       []int
	mutex_activeElevators sync.Mutex
)

var (
	backupStates [numElev]ElevState // The backup states array
	mutex_backup sync.Mutex
)

var (
	isWaiting     bool
	mutex_waiting sync.Mutex
)

var mutex_d sync.Mutex // Mutex for the direction of the elevator

var lastDirForStopFunction elevio.MotorDirection // The last direction the elevator was moving in before the stop button was pressed

// Variables for the MotorStop
const timerHallOrder time.Duration = 10 * time.Second   // Assuming 30 seconds for the timer
const pollRateMotorStop time.Duration = 3 * time.Second // The rate at which we check for power shortage

var mutex_lastSeenMotorStop sync.Mutex // Mutex for the lastSeen variable in detectMotorStop
var mutex_elevatorOrdersMotorStop sync.Mutex
