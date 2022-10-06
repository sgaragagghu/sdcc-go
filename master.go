package main

import (
	"net"
	"net/rpc"
	"reflect"
	"time"

	"github.com/elliotchance/orderedmap"
)

const MASTER_PORT = "9999"

func heartbeat_goroutine() {

	InfoLoggerPtr.Println("Heartbeat_goroutine started.")

	linked_hashmap := orderedmap.NewOrderedMap()

	for {
		select {
		case heartbeat_ptr := <-*Heartbeat_channel_ptr:
			element_temp := linked_hashmap.GetElement(heartbeat_ptr.Id)
			var server_temp_ptr *Server
			if element_temp != nil {
				server_temp_ptr = element_temp.Value.(*Server)
				server_temp_ptr.Last_heartbeat = heartbeat_ptr.Last_heartbeat
				if !linked_hashmap.Delete(heartbeat_ptr.Id) { // TODO Check: could work without deletion
					ErrorLoggerPtr.Fatal("Unexpected error")
				}
			} else {
				// TODO write on chan that there's a new server
				*server_temp_ptr = *heartbeat_ptr
			}
			linked_hashmap.Set(server_temp_ptr.Id, *server_temp_ptr) // TODO is it efficient ? 
			InfoLoggerPtr.Println("received heartbeat")
		default:
			for el := linked_hashmap.Front(); el != nil;  {
				server_temp_ptr := el.Value.(*Server)
				el = el.Next()
				if server_temp_ptr.Last_heartbeat.Unix() > time.Now().Unix() - EXPIRE_TIME {
					if !linked_hashmap.Delete(server_temp_ptr.Id) {
						ErrorLoggerPtr.Fatal("Unexpected error")
					}
					// TODO write on chan that this server is dead
				}
			}
			time.Sleep(1)
		}
	}
}

func master_main() {

	// creating channel for communicating the heartbeat
	// to the goroutine heartbeat manager
	Heartbeat_channel_ptr = new(chan *Server)

	go heartbeat_goroutine()

	master_handler := new(Master_handler)

	// register Master_handler as RPC interface
	rpc.Register(master_handler)

	// service address of server
	service := ":" + MASTER_PORT

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
		go rpc.ServeConn(conn)
	}

}

