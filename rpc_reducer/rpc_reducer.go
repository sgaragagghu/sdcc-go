package rpc_reducer

import (
	. "../share"
//	. "../rpc_mapper"
//	"container/list"
)

const REDUCER_PORT = "6665"

var (
	Job_reducer_channel chan *Job
	Job_full_channel chan *Request
)

// create a type to get an interface
type Reducer_handler int

func (h Reducer_handler) Send_job(args *Job, reply *int) error {
	Job_reducer_channel <- args
	return nil
}

func (h Reducer_handler) Send_job_full(args *Request, reply *interface{}) error {
	Job_full_channel <- args
	return nil
}

