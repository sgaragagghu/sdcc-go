package main

import (
	"time"
)

const EXPIRE_TIME = 10

var (
	Heartbeat_channel_ptr *<-chan *Server
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

