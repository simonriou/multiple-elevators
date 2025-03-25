package peers

import (
	"Network-go/network/conn"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"time"
)

/*
### Updates made to this file ###
- Changed the Transmitter function so that it sends the ElevIdentity instead of a string
	This allows us to send both the ID (fixed) and the role (variable) of an elevator
- Had to change the Receiver function so that it is only detecting peer losses depending on their ID and not
	on the entire ElevIdentity struct
*/

type ElevIdentity struct {
	Id   int    `json:"ID"`
	Role string `json:"Role"`
}

type PeerUpdate struct {
	Peers []ElevIdentity
	New   ElevIdentity
	Lost  []ElevIdentity
}

const interval = 15 * time.Millisecond
const timeout = 500 * time.Millisecond

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

	lastSeen := make(map[int]time.Time)        // Track last seen time by Id
	idToIdentity := make(map[int]ElevIdentity) // Track latest ElevIdentity by Id

	conn := conn.DialBroadcastUDP(port)

	for {
		updated := false
		var p PeerUpdate

		conn.SetReadDeadline(time.Now().Add(interval))
		n, _, err := conn.ReadFrom(buf[0:])
		if err != nil {
			// Read timeout; continue to check for lost peers
		} else {
			var receivedID ElevIdentity
			err := json.Unmarshal(buf[:n], &receivedID)
			if err == nil {
				id := receivedID.Id
				_, exists := lastSeen[id]

				// If this is a new peer (new ID)
				if !exists {
					p.New = receivedID
					updated = true
				} else {
					p.New = ElevIdentity{} // Zero value if not new
				}

				// Update the last seen time and identity for this id
				lastSeen[id] = time.Now()
				idToIdentity[id] = receivedID
			}
		}

		// Detect lost peers
		p.Lost = make([]ElevIdentity, 0)
		now := time.Now()
		for id, t := range lastSeen {
			if now.Sub(t) > timeout {
				updated = true
				// Append the last known identity before deletion
				p.Lost = append(p.Lost, idToIdentity[id])
				delete(lastSeen, id)
				delete(idToIdentity, id)
			}
		}

		if updated {
			// Build the current peer list from idToIdentity
			p.Peers = make([]ElevIdentity, 0, len(idToIdentity))
			for _, identity := range idToIdentity {
				p.Peers = append(p.Peers, identity)
			}

			// Optional: sort peers and lost for deterministic output
			sort.Slice(p.Peers, func(i, j int) bool {
				return p.Peers[i].Id < p.Peers[j].Id
			})
			sort.Slice(p.Lost, func(i, j int) bool {
				return p.Lost[i].Id < p.Lost[j].Id
			})

			peerUpdateCh <- p
		}
	}
}
