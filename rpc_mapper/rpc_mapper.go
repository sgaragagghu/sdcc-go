package rpc_mapper

import (
	. "../share"
//	"time"
//	. "../rpc_master"
//	"container/list"
)

const MAPPER_PORT = "6668"

var (
	Job_channel chan *Job
	Job_full_request_channel chan *Request
)

// create a type to get an interface
type Mapper_handler int

func (h Mapper_handler) Send_job(args *Job, reply *int) error {
	Job_channel <- args
	return nil
}

func (h Mapper_handler) Get_job_full(args *Request, reply *int) error {
	Job_full_request_channel <- args
	return nil
}
