package main

import (
	"time"
)

// create a type to get an interface
type Master_handler int

func (h Master_handler) Send_heartbeat(args *Server, reply *int) error {
	InfoLoggerPtr.Println("OK!")
	return nil
}

type Server struct {
	Id string
	Ip string
	Port string
	Last_heartbeat time.Time
	Role string
}

