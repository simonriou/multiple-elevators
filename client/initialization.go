package main

import (
	"Driver-go/elevio"
	"Network-go/network/peers"
	"flag"
	"fmt"
	"os"
)

func isIDValid(id int, elevators peers.PeerUpdate) bool {
	newElev := elevators.New
	oldElevs := elevators.Peers
	// Remove the new elevator from the list of old elevators
	for i, elev := range oldElevs {
		if elev == newElev {
			oldElevs = append(oldElevs[:i], oldElevs[i+1:]...)
			break
		}
	}

	// Check if the ID is already taken
	for _, elev := range oldElevs {
		if elev.Id == id {
			return false
		}
	}

	return true
}

func initSingleElev(d elevio.MotorDirection, drv_floors chan int) {
	drv_finishedInitialization := make(chan bool)
	turnOffAllLights()
	go func() {
		elevio.SetMotorDirection(d)
		for {
			a := <-drv_floors
			fmt.Printf("%v\n", a)
			if a == 0 {
				d = elevio.MD_Stop
				elevio.SetMotorDirection(d)
				break
			}
		}
		//fmt.Println("Found 0 floor")
		ableToCloseDoors = true
		//turnOffLights(Order{0, -1, 0}, true)

		drv_finishedInitialization <- true
	}()

	<-drv_finishedInitialization

	fmt.Printf("Initialization finished\n")
}

func getFlags() (string, string, int) {
	// Decide the port on which we are working (for the server) & the role of the elevator
	port_raw := flag.String("port", "", "The port of the elevator client / server")
	role_raw := flag.String("role", "", "The role of the elevator")
	id_raw := flag.Int("id", -1, "The id of the elevator")
	flag.Parse()

	port := *port_raw
	role = *role_raw
	id := *id_raw

	// If role is different that Regular, Master or PrimaryBackup, cancel the program
	if role != "Regular" && role != "Master" && role != "PrimaryBackup" {
		fmt.Println("Role must be either Regular, Master or PrimaryBackup")
		os.Exit(1)
	}

	// If the ID is not an integer, cancel the program
	if id < 0 {
		fmt.Println("ID must be a positive integer")
		os.Exit(1)
	}

	// If the port is not a number, cancel the program
	if port == "" {
		fmt.Println("Port must be a number")
		os.Exit(1)
	}

	//fmt.Printf("Working on address: %v\n", "localhost:"+port)
	//fmt.Printf("Role passed: %v\n", role)
	//fmt.Printf("Id passed: %v\n", id)

	return port, role, id
}
