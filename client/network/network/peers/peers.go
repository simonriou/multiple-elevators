package peers

import (
	"Network-go/network/conn"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"time"
)

type PeerUpdate struct {
	Peers []string
	New   string
	Lost  []string
}

// Define the struct to send
type ElevIdentity struct {
	Id   int    `json:"ID"`
	Role string `json:"Role"`
}

const interval = 15 * time.Millisecond
const timeout = 500 * time.Millisecond

// // Transmitter sends a serialized Payload struct via UDP
// func Transmitter(port int, id int, role string, transmitEnable <-chan bool) {
// 	conn := conn.DialBroadcastUDP(port)
// 	addr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", port))

// 	enable := true
// 	interval := time.Second // define interval if not already declared

// 	// Create a reusable message structure
// 	msg := ElevIdentity{Id: id, Role: role}

// 	for {
// 		select {
// 		case enable = <-transmitEnable:
// 		case <-time.After(interval):
// 		}

// 		if enable {
// 			// Serialize the struct into bytes (JSON)
// 			data, err := json.Marshal(msg)
// 			if err != nil {
// 				continue // or handle the error
// 			}
// 			conn.WriteTo(data, addr)
// 		}
// 	}
// }

func Transmitter(port int, id int, roleChan <-chan string, transmitEnable <-chan bool) {
	conn := conn.DialBroadcastUDP(port)
	addr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", port))

	enable := true
	currentRole := ""
	msg := ElevIdentity{Id: id, Role: currentRole}

	for {
		select {
		case enable = <-transmitEnable:
		case newRole := <-roleChan:
			currentRole = newRole
			msg.Role = currentRole
		case <-time.After(interval):
		}
		if enable {
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			conn.WriteTo(data, addr)
		}
	}
}

func Receiver(port int, peerUpdateCh chan<- PeerUpdate) {

	var buf [1024]byte
	var p PeerUpdate
	lastSeen := make(map[string]time.Time)

	conn := conn.DialBroadcastUDP(port)

	for {
		updated := false

		conn.SetReadDeadline(time.Now().Add(interval))
		n, _, _ := conn.ReadFrom(buf[0:])

		id := string(buf[:n])

		// Adding new connection
		p.New = ""
		if id != "" {
			if _, idExists := lastSeen[id]; !idExists {
				p.New = id
				updated = true
			}

			lastSeen[id] = time.Now()
		}

		// Removing dead connection
		p.Lost = make([]string, 0)
		for k, v := range lastSeen {
			if time.Now().Sub(v) > timeout {
				updated = true
				p.Lost = append(p.Lost, k)
				delete(lastSeen, k)
			}
		}

		// Sending update
		if updated {
			p.Peers = make([]string, 0, len(lastSeen))

			for k, _ := range lastSeen {
				p.Peers = append(p.Peers, k)
			}

			sort.Strings(p.Peers)
			sort.Strings(p.Lost)
			peerUpdateCh <- p
		}
	}
}
