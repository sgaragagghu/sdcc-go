package main

import (
//	"fmt"
	"os"
	"time"
	"net/rpc"
)

var (
	server *Server
)

const MAPPER_PORT = "6667"

func heartbeat(client *rpc.Client) {

	var (
		reply	int
		err	error
	)

	for {
		server.Last_heartbeat = time.Now()
		err = client.Call("Master_handler.Send_heartbeat", server, &reply)
		if err != nil {
			ErrorLoggerPtr.Fatal("Heartbeat error:", err)
		}
		time.Sleep(EXPIRE_TIME / 2)
	}

}

func init() {
	id := os.Getenv("UUID") + APP_ID 
	if id == APP_ID {
		ErrorLoggerPtr.Fatal("There is no UUID")
	}

	ip := GetOutboundIP().String()

	server = &Server{id, ip, MAPPER_PORT, time.Now(), "MAPPER"}
}

func mapper_main() {

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", MASTER_IP + ":" + MASTER_PORT)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	go heartbeat(client)

}

