package rpc_reducer

import (
//	. "../share"
//	"container/list"
)

const REDUCER_PORT = "6665"

var (
	Job_reducer_channel chan *Job
)

// create a type to get an interface
type Reducer_handler int

func (h Reducer_handler) Send_job(args *Job, reply *int) error {
	Job_reducer_channel <- args
	return nil
}

