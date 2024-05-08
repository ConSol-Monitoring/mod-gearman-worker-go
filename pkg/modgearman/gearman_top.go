package modgearman

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// func printStatus(queues [][]string) {
// 	// Find longest queue name if the name is longer than the column description
// 	maxLength := len("Queue Name")
// 	for i, queue := range queues {
// 		if len(queues[i]) < 4 {
// 			continue
// 		}
// 		queueNameLen := len(queue[0])
// 		if queueNameLen > maxLength {
// 			maxLength = queueNameLen
// 		}
// 	}
// 	fmt.Println(maxLength)

// 	// Find queue name length difference
// 	defaultLen := len("Queue Name")
// 	runeDiff := 0
// 	if maxLength > defaultLen {
// 		runeDiff = maxLength - defaultLen
// 	}

// 	// Print table
// 	// First Line
// 	fmt.Printf(" Queue Name")

// }

func GetGearmanServerData(hostname string, port int) [][]string {
	var gearmanStatus string = Send2gearmandadmin("status\nversion\n", hostname, port)

	if gearmanStatus == "" {
		return nil
	}

	// Organize queues into a list
	var queues [][]string
	lines := strings.Split(gearmanStatus, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		fmt.Println(parts)
		fmt.Println(len(parts))
		queues = append(queues, parts)
	}

	return queues
}

func Send2gearmandadmin(cmd string, hostname string, port int) string {
	conn, connErr := gm_net_connect(hostname, port)
	if connErr != nil {
		fmt.Println(connErr)
		return ""
	}

	_, writeErr := conn.Write([]byte(cmd))
	if writeErr != nil {
		fmt.Println(writeErr)
		return ""
	}

	// Read response
	//var buffer []byte
	buffer := make([]byte, 512)
	n, readErr := conn.Read(buffer)
	if readErr != nil {
		fmt.Println(readErr)
		return ""
	} else {
		result := string(buffer[:n])
		fmt.Println(result)
		return result
	}
}

func gm_net_connect(hostname string, port int) (net.Conn, error) {
	addr := hostname + ":" + strconv.Itoa(port)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	// Success
	return conn, nil
}
