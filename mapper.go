package main

import (
	"fmt"
	"os"
	"io/ioutil"
	"strconv"
	"time"
	"net/rpc"
	"crypto/sha256"
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

	ip := GetOutboundIP().String()
	var id string

	bytes, err := ioutil.ReadFile("./ID")
	if string(bytes) == "" || err != nil {

		id = ip + MAPPER_PORT + APP_ID + strconv.FormatInt(time.Now().Unix(), 10)
		id = fmt.Sprintf("%x", sha256.Sum256([]byte(id)))

		file, err := os.Create("./ID")
		if err != nil {
			ErrorLoggerPtr.Fatal("Cannot create ID file.", err)
		}
		file.WriteString(id)
		file.Close()
	} else {
		id = string(bytes)
	}

	if id == "" {
		ErrorLoggerPtr.Fatal("Empty ID.", err)
	}

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

