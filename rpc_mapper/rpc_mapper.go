package rpc_mapper

import (
//	. "../share"
//	"container/list"
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
	Margin int8
	Separate_entries byte
	Separate_properties byte
	Properties_amount int8
	Map_algorithm string
	Map_algorithm_parameters interface{}
	Result map[string]interface{}
}


func (h Mapper_handler) Send_job(args *Job, reply *int) error {
	*Job_channel_ptr <- args
	return nil
}

