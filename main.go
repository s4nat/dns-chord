package main

import (
	"bufio"
	"core/node"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/joho/godotenv"
)

// Color coded logs
var system = color.New(color.FgCyan).Add(color.BgBlack)

/*
Show a list of options to choose from.
*/
func showmenu() {
	system.Println("********************************")
	system.Println("\t\tMENU")
	system.Println("Press 1 to see the fingertable")
	system.Println("Press 2 to see the successor and predecessor")
	system.Println("Press m to see the menu")
	system.Println("********************************")
}

func main() {
	// get port from cli arguments (specified by user)
	err := godotenv.Load()
	if err != nil {
		system.Println("Error getting env variables...")
	}

	var port string
	var helperIp string

	// Read your own port number and also the IP address of the other node, if new network
	myIpAddress := GetOutboundIP().String()
	reader := bufio.NewReader(os.Stdin)
	// read input from user
	system.Print("Enter your port number:")
	port, err = reader.ReadString('\n')
	if err != nil {
		system.Fprintln(os.Stderr, "Error reading input:", err)
	}
	system.Println("Enter IP address and port used to join network:")
	// read input from user
	helperIp, err = reader.ReadString('\n')
	if err != nil {
		system.Fprintln(os.Stderr, "Error reading input:", err)
	}

	// Create new Node object for yourself
	me := node.Node{}
	var addr = myIpAddress + ":" + port
	system.Println(addr)
	me.IP = addr[:len(addr)-1]
	me.Nodeid = GenerateHash(addr)
	system.Println("My id is:", me.Nodeid)

	// Bind yourself to a port and listen to it
	tcpAddr, err := net.ResolveTCPAddr("tcp", me.IP)
	if err != nil {
		system.Println("Error resolving TCP address", err)
	}
	inbound, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		system.Println("Could not listen to TCP address", err)
	}

	// Register RPC methods and accept incoming requests
	rpc.Register(&me)
	system.Println("Node is runnning at IP address:", tcpAddr)
	go rpc.Accept(inbound)

	// Join the network using helperIp
	me.JoinNetwork(helperIp[:len(helperIp)-1])

	showmenu()
	// Keep the parent thread alive
	for {
		time.Sleep(1000)
		var input string
		fmt.Scanln(&input)
		if input == "1" {
			me.PrintFingers()
		} else if strings.HasPrefix(input, "query") {
			system.Print("Please Type the Website: ")
			fmt.Scanln(&input)
			me.QueryDNS(input)
		} else if strings.ToLower(input) == "m" {
			showmenu()
		} else if input == "2" {
			system.Println("\n\nSuccessor")
			me.PrintSuccessor()
			system.Println("Predecessor")
			me.PrintPredecessor()
		}
	}

}
