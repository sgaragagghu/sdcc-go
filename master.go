package main

import (
//	"fmt"
//	"os"
//	"time"
//	"log"
	"net"
	"net/rpc"
	"reflect"
)

func master_main() {

	master_handler := new(Master_handler)

	// register Master_handler as RPC interface
	rpc.Register(master_handler)

	// service address of server
	service := ":1200"

	// create tcp address
	tcpAddr, err := net.ResolveTCPAddr("tcp", service)
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	// tcp network listener
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	for {
		// handle tcp client connections
		conn, err := listener.Accept()
		if err != nil {
			WarningLoggerPtr.Println("listener accept error:", err)
		}

		// print connection info
		InfoLoggerPtr.Println("received message", reflect.TypeOf(conn), conn)

		// handle client connections via rpc
		rpc.ServeConn(conn)
	}

}

