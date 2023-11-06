/*
The provided codebase implements a simplified Chord protocol, a decentralized peer-to-peer (P2P) distributed hash table (DHT)
for distributed data storage and lookup. The system orchestrates nodes forming a ring-based network structure where each node
maintains information about its successor, predecessor, and a portion of the network keyspace. It includes functionalities for
node joining, stabilizing the network, and updating finger tables, enabling efficient decentralized lookup of key-value pairs
across a distributed system where each node manages a segment of the overall keyspace. The implementation involves periodic
checks, such as node stabilization, finger table fixing, predecessor checks, and message handling for essential network operations
like finding successors and notifying or updating neighboring nodes.
*/
package node

import (
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"time"

	"core.com/message"
	"core.com/utility"
	"github.com/fatih/color"
)

// Colour coded logs
var system = color.New(color.FgHiGreen).Add(color.BgBlack)
var systemcommsin = color.New(color.FgHiMagenta).Add(color.BgBlack)
var systemcommsout = color.New(color.FgHiYellow).Add(color.BgBlack)

type Pointer struct {
	Nodeid uint64 // ID of the pointed Node
	IP     string // IP of the pointed Node
}
type Cache struct {
	value   []string
	counter uint64
}
type Node struct {
	Nodeid        uint64              // ID of the node
	IP            string              // localhost or IP address AND port number. Can be set through environment variables.
	FingerTable   []Pointer           // id mapping to ip address
	Successor     Pointer             // Nodeid of it's direct successor.
	Predecessor   Pointer             // Nodeid of it's direct predecessor.
	Logging       bool                // logging for messages
	CachedQuery   map[uint64]Cache    // caching queries on the node locally
	HashIPStorage map[uint64][]string // storage for hashed ips associated with the node
	Counter       uint64
}

// Constants
const M = 32
const CACHE_SIZE = 5

// Message types
const PING = "ping"
const ACK = "ack"
const FIND_SUCCESSOR = "find_successor"
const CLOSEST_PRECEDING_NODE = "closest_preceding_node"
const GET_PREDECESSOR = "get_predecessor"
const NOTIFY = "notify"
const PUT = "put"
const GET = "get"

/*
The default method called by all RPCs. This method receives different
types of requests, and calls the appropriate functions.
*/
func (node *Node) HandleIncomingMessage(msg *message.RequestMessage, reply *message.ResponseMessage) error {
	if node.Logging {
		systemcommsin.Println("Message of type", msg.Type, "received.")
	}
	switch msg.Type {
	case PING:
		systemcommsin.Println("Received ping message")
		reply.Type = ACK
	case FIND_SUCCESSOR:
		if node.Logging {
			systemcommsin.Println("Received a message to find successor of", msg.TargetId)
		}
		pointer := node.FindSuccessor(msg.TargetId)
		reply.Type = ACK
		reply.Nodeid = pointer.Nodeid
		reply.IP = pointer.IP
	case NOTIFY:
		if node.Logging {
			systemcommsin.Println("Received a message to notify me about a new predecessor", msg.TargetId)
		}
		status := node.Notify(Pointer{Nodeid: msg.TargetId, IP: msg.IP})
		if status {
			reply.Type = ACK
		}
	case GET_PREDECESSOR:
		if node.Logging {
			systemcommsin.Println("Received a message to get predecessor")
		}
		reply.Nodeid = node.Predecessor.Nodeid
		reply.IP = node.Predecessor.IP
	default:
		// system.Println("Client is alive and listening")
		time.Sleep(1000)
	}
	return nil
}

/*
When a node first joins, it checks if it is the first node, then creates a new
chord network, or joins an existing chord network accordingly.
*/
func (node *Node) JoinNetwork(helper string) {
	if len(strings.Split(helper, ":")) == 1 { // I am the only node in this network
		system.Println("I am creating a new network...")
		node.Successor = Pointer{Nodeid: node.Nodeid, IP: node.IP}
		node.Predecessor = Pointer{}
		node.FingerTable = make([]Pointer, M)
		go node.FixFingers()
		system.Println("Finger table has been updated...")
		for i := 0; i < len(node.FingerTable); i++ {
			system.Printf("> Finger[%d]: %d : %s\n", i+1, node.FingerTable[i].Nodeid, node.FingerTable[i].IP)
		}
	} else { // I am not the only one in this network, and I am joining using someone elses address-> "helper"
		system.Println("Contacting node in network at address", helper)
		reply := node.CallRPC(message.RequestMessage{Type: FIND_SUCCESSOR, TargetId: node.Nodeid}, helper)
		node.Successor = Pointer{Nodeid: reply.Nodeid, IP: reply.IP}
		system.Println("My successor id is:", node.Successor.Nodeid)
		node.Predecessor = Pointer{}
		node.FingerTable = make([]Pointer, M)
		go node.FixFingers()
		system.Println("Finger table has been updated...")
		for i := 0; i < len(node.FingerTable); i++ {
			system.Printf("> Finger[%d]: %d : %s\n", i+1, node.FingerTable[i].Nodeid, node.FingerTable[i].IP)
		}
	}
	time.Sleep(2 * time.Second)
	go node.stabilize()
	go node.CheckPredecessor()
}

/*
If id falls between its successor, find successor is finished and node
n returns its successor. Otherwise, n searches its finger table for the
node whose ID most immediately precedes id, and then invokes find successor
at that ID
*/
func (node *Node) FindSuccessor(id uint64) Pointer {
	if belongsTo(id, node.Nodeid, node.Successor.Nodeid) {
		return Pointer{Nodeid: node.Successor.Nodeid, IP: node.Successor.IP} // Case when this is the first node.
	}
	p := node.ClosestPrecedingNode(id)
	if (p != Pointer{} && p.Nodeid != node.Nodeid) {
		reply := node.CallRPC(message.RequestMessage{Type: FIND_SUCCESSOR, TargetId: id}, p.IP)
		return Pointer{Nodeid: reply.Nodeid, IP: reply.IP}
	} else {
		return node.Successor
	}
}

/*
Works jointly with FindSuccessor(id). If id doesn't fall between
my id, and my immediate successors id, then we find the closest
preceding node, so we can call find successor on that node.
*/
func (node *Node) ClosestPrecedingNode(id uint64) Pointer {
	for i := M - 1; i >= 0; i-- {
		if belongsTo(node.FingerTable[i].Nodeid, node.Nodeid, id) {
			return node.FingerTable[i]
		}
	}
	system.Println("Closes Preceding node outside fingertable:", Pointer{Nodeid: node.Nodeid, IP: node.IP})
	return Pointer{Nodeid: node.Nodeid, IP: node.IP}
}

/*
Each node periodically calls fix fingers to make sure its finger
table entries are correct; this is how new nodes initialize
their finger tables, and it is how existing nodes incorporate
new nodes into their finger tables.
*/
func (node *Node) FixFingers() {

	for {
		time.Sleep(5 * time.Second)
		system.Println("Fixing fingers...")
		for id := range node.FingerTable {
			nodePlusTwoI := (node.Nodeid + uint64(math.Pow(2, float64(id))))
			power := uint64(math.Pow(2, float64(M)))
			if nodePlusTwoI > power {
				nodePlusTwoI -= power
			}
			node.FingerTable[id] = node.FindSuccessor(uint64(nodePlusTwoI))
		}
	}
}

/*
Every node runs stabilize() periodically to learn about newly
joined nodes. Each time node n runs stabilize(), it asks its successor
for the successor’s predecessor p, and decides whether p
should be n’s successor instead. This would be the case if node p
recently joined the system. In addition, stabilize() notifies node
n’s successor of n’s existence, giving the successor the chance
to change its predecessor to n. The successor does this only if it
knows of no closer predecessor than n.
*/
func (node *Node) stabilize() {
	for {
		time.Sleep(5 * time.Second)
		reply := node.CallRPC(
			message.RequestMessage{Type: GET_PREDECESSOR, TargetId: node.Successor.Nodeid, IP: node.Successor.IP},
			node.Successor.IP,
		)
		sucessorsPredecessor := Pointer{Nodeid: reply.Nodeid, IP: reply.IP}
		if (sucessorsPredecessor != Pointer{}) {
			// The new dude in between you and your successor is not dead, then my true successor is the new dude. Or you're the only dude.
			if between(sucessorsPredecessor.Nodeid, node.Nodeid, node.Successor.Nodeid) {
				node.Successor = Pointer{Nodeid: sucessorsPredecessor.Nodeid, IP: sucessorsPredecessor.IP}
			}
		} else {
			node.Successor = node.FindSuccessor(node.Nodeid)
			if (node.Successor == Pointer{}) {
				node.Successor = Pointer{Nodeid: node.Nodeid, IP: node.IP}
			}
		}
		if node.Nodeid != node.Successor.Nodeid {
			reply := node.CallRPC(
				message.RequestMessage{Type: NOTIFY, TargetId: node.Nodeid, IP: node.IP},
				node.Successor.IP,
			)
			if reply.Type == ACK {
				system.Println("Successfully notified successor of it's new predecessor")
			}
		}
	}
}

/*
x thinks it might be nodes predecessor
*/
func (node *Node) Notify(x Pointer) bool {
	if (node.Predecessor == Pointer{} || between(x.Nodeid, node.Predecessor.Nodeid, node.Nodeid)) {
		node.Predecessor = Pointer{Nodeid: x.Nodeid, IP: x.IP}
		return true
	}
	return false
}

/*
Each node also runs check predecessor periodically, to clear the node’s
predecessor pointer if n.predecessor has failed; this allows it to accept
a new predecessor in notify.
*/
func (node *Node) CheckPredecessor() {
	for {
		time.Sleep(5 * time.Second)
		if (node.Predecessor == Pointer{}) {
			continue
		}
		system.Println("I came")
		reply := node.CallRPC(message.RequestMessage{Type: PING}, node.Predecessor.IP)
		if (reply == message.ResponseMessage{}) {
			node.Predecessor = Pointer{}
		} else {
			system.Println("Predecessor", node.Predecessor.IP, "is alive")
		}
	}
}

func (node *Node) QueryDNS(website string) {
	if node.CachedQuery == nil {
		node.CachedQuery = make(map[uint64]Cache)
	}
	node.Counter += 1

	if strings.HasPrefix(website, "www.") {
		system.Println("> Removing Prefix")
		website = website[4:]
	}
	hashedWebsite := utility.GenerateHash(website)
	system.Printf("> The Website %s has been hashed to %d\n", website, hashedWebsite)
	succPointer := node.FindSuccessor(hashedWebsite)
	system.Printf(">  The Website would be stored at it's succesor %d : %s\n", succPointer.Nodeid, succPointer.IP)
	ip_addr, ok := node.CachedQuery[hashedWebsite]
	if ok {
		system.Println("> Retrieving from Cache")
		for _, ip_c := range ip_addr.value {
			system.Printf("> %s. IN A %s\n", website, ip_c)
		}
	} else {
		ips, err := net.LookupIP(website)
		if err != nil {
			system.Printf("> Could not get IPs: %v\n", err)
			os.Exit(1)
		}
		ip_addresses := []string{}
		for _, ip := range ips {
			ip_addresses = append(ip_addresses, ip.String())
			system.Printf("> %s. IN A %s\n", website, ip.String())
		}
		node.CachedQuery[hashedWebsite] = Cache{value: ip_addresses, counter: node.Counter}
		if len(node.CachedQuery) > CACHE_SIZE {
			var minKey uint64
			minValue := uint64(18446744073709551615)
			for key, value := range node.CachedQuery {
				if value.counter < minValue {
					minKey = key
					minValue = value.counter
				}
			}
			if minKey != 0 {
				delete(node.CachedQuery, minKey)
			}

		}
		node.writeToStorage(hashedWebsite, ip_addresses)
		for key, value := range node.CachedQuery {
			system.Printf("Key: %d, Value: %s, %d\n", key, value.value[0], value.counter)
		}

	}
	// node.CachedQuery[website] = ip.String();

}

func (node *Node) writeToStorage(hashedWebsite uint64, ip_addresses []string) {
	filePath := "/app/data/example.txt"
	content := fmt.Sprintf("%d : %v\n", hashedWebsite, ip_addresses)

	// Write to the file, create it if it doesn't exist
	// Append to the file or create it if it doesn't exist
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Error opening or creating the file: %v\n", err)
		return
	}
	defer file.Close()

	// Write the content to the file
	_, err = file.WriteString(content)
	if err != nil {
		fmt.Printf("Error writing to the file: %v\n", err)
		return
	}

	fmt.Printf("Appended to file: %s\n", filePath)

	// Read the contents of the file
	file, err = os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening the file for reading: %v\n", err)
		return
	}
	defer file.Close()

	// Read the file contents
	buffer := make([]byte, 1024)
	n, err := file.Read(buffer)
	if err != nil {
		fmt.Printf("Error reading the file: %v\n", err)
		return
	}

	fileContents := string(buffer[:n])
	fmt.Printf("File contents:\n%s\n", fileContents)

}
