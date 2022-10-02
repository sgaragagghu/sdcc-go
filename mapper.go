package main

import (
	"fmt"
//	"os"
	"time"
	"net/rpc"
)

func mapper_main() {

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", ":1200")
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	// reply is used to hold responses
	var reply int

	// RPC call (1)
	// remote call procedure Person.FirstLast (FirstLast method on Person object)
	err = client.Call("Master_handler.Send_heartbeat", &Server{"", "", "", time.Now(), ""}, &reply)
	if err != nil {
		ErrorLoggerPtr.Fatal("Person.FirstLast error:", err)
	}
	fmt.Printf("Response: %d\n", reply)

}

