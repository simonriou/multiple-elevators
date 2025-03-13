package main

import "Driver-go/elevio"

func listContains(list []string, element string) bool {
	for _, e := range list {
		if e == element {
			return true
		}
	}
	return false
}

func decideNewRoleLoss(role string, remainingRoles []string) string {
	switch {
	case role == "PrimaryBackup" && !listContains(remainingRoles, "Master"): // Case where Backup -> Master
		return "Master"
	case role == "Regular" && !listContains(remainingRoles, "Master"): // Case where Slave -> Backup
		return "PrimaryBackup"
	case role == "Regular" && !listContains(remainingRoles, "PrimaryBackup"): // Case where Slave -> Backup
		return "PrimaryBackup"
	}
	return role
}

func startRoutine(role string,
	hallBtnRx chan elevio.ButtonEvent,
	singleStateRx chan StateMsg,
	hallOrderTx chan HallOrderMsg,
	allStatesTx chan [numElev]StateMsg) {
	switch role {
	case "Master":
		go MasterRoutine(hallBtnRx, singleStateRx, hallOrderTx, allStatesTx)
	case "PrimaryBackup":
		go PrimaryRoutine()
	}
}
