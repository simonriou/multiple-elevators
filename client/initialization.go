package main

import (
	"Driver-go/elevio"
	"flag"
	"fmt"
)

func initSingleElev(d elevio.MotorDirection, drv_floors chan int) {
	drv_finishedInitialization := make(chan bool)
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

	//fmt.Printf("Working on address: %v\n", "localhost:"+port)
	//fmt.Printf("Role passed: %v\n", role)
	//fmt.Printf("Id passed: %v\n", id)

	return port, role, id
}
