package main

import (
	"net"
	"net/rpc"
	"reflect"

	"github.com/elliotchance/orderedmap"
)

var (
	Heartbeat_channel_ptr *<-chan Server
)

func heartbeat_goroutine() {

	InfoLoggerPtr.Println("Heartbeat_goroutine started.")

	linked_hashmap := orderedmap.NewOrderedMap()

	for {
		select {
		case heartbeat := <-*Heartbeat_channel_ptr:
			element_temp := linked_hashmap.GetElement(heartbeat.Id)
			var server_temp_ptr *Server
			if element_temp != nil {
				server_temp_ptr = element_temp.Value.(*Server)
				server_temp_ptr.Last_heartbeat = heartbeat.Last_heartbeat
				linked_hashmap.Delete(heartbeat.Id) // TODO check error (COPIARE PRIMA DI ELIMINARE.... memcp)
			} else {
				// TODO build the server...
				server_temp_ptr.Last_heartbeat = heartbeat.Last_heartbeat
			}
			linked_hashmap.Set(server_temp_ptr.Id, *server_temp_ptr) // TODO check error (e anche controllare che non serve in realta' il puntatore..) 
			InfoLoggerPtr.Println("received heartbeat")
		default:
		}

		// TODO controllare le scadenze
	}
}

func master_main() {

	// creating channel for communicating the heartbeat
	// to the goroutine heartbeat manager
	Heartbeat_channel_ptr = new(<-chan Server)

	go heartbeat_goroutine()

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

