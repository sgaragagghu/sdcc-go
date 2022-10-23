package rpc_master

import (
	. "../share"
//	. "../rpc_mapper"
//	"time"
)

const (
	EXPIRE_TIME = 10
	MASTER_PORT = "9999"
)

var (
	Heartbeat_channel chan *Server
	Job_mapper_completed_channel chan *Job
	Job_reducer_completed_channel chan *Job
)

// create a type to get an interface
type Master_handler int

func (h Master_handler) Send_heartbeat(args *Server, reply *int) error {
	Heartbeat_channel <- args
	return nil
}

func (h Master_handler) Job_mapper_completed(args *Job, reply *int) error {
	Job_mapper_completed_channel <- args
	return nil
}

func (h Master_handler) Job_reducer_completed(args *Job, reply *int) error {
	Job_reducer_completed_channel <- args
	return nil
}
