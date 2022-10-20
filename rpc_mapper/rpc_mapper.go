package rpc_mapper

import (
//	. "../share"
//	"container/list"
)

const MAPPER_PORT = "6668"

var (
	Job_mapper_channel chan *Job
)

// create a type to get an interface
type Mapper_handler int

func (h Mapper_handler) Send_job(args *Job, reply *int) error {
	Job_mapper_channel <- args
	return nil
}

