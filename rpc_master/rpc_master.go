package rpc_master

import (
//	. "../share"
	. "../rpc_mapper"
	"time"
)

const (
	EXPIRE_TIME = 10
	MASTER_PORT = "9999"
)

var (
	Heartbeat_channel_ptr *chan *Server
	Job_completed_channel_ptr *chan *Job
)

// create a type to get an interface
type Master_handler int

type Server struct {
	Id string
	Ip string
	Port string
	Last_heartbeat time.Time
	Jobs *map[string]*Job
	Role string
}


func (h Master_handler) Send_heartbeat(args *Server, reply *int) error {
	*Heartbeat_channel_ptr <- args
	return nil
}

func (h Master_handler) Job_completed(args *Job, reply *int) error {
	*Job_completed_channel_ptr <- args
	return nil
}

