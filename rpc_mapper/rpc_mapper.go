package rpc_mapper

import (
//	. "../share"
)

const MAPPER_PORT = "6668"

var (
	Job_channel_ptr *chan *Job
)

// create a type to get an interface
type Mapper_handler int

type Job struct {
	Id string
	Task_id string
	Server_id string
	Resource_link string
	Begin int64
	End int64
	Margin int32
	Separate_entries byte
	Separate properties byte
	Map_algorithm string
}


func (h Mapper_handler) Send_task(args *Job, reply *int) error {
	*Job_channel_ptr <- args
	return nil
}

