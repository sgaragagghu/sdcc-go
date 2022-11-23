package rpc_mapper

import (
	. "../share"
//	"time"
//	. "../rpc_master"
//	"container/list"
)

const MAPPER_PORT = "6668"

var (

	This_server *Server
	Job_channel chan *Job
	Job_full_request_channel chan *Request
	Task_completed_channel chan map[string]struct{}
)

// create a type to get an interface
type Mapper_handler int
// Used by the master ti send jobs to the mapper
func (h Mapper_handler) Send_job(args *Job, reply *int) error {
	Job_channel <- args
	return nil
}
// Used by the master to notify that a task has been fully completed and its data can be deleted
func (h Mapper_handler) Task_completed(args *Request, reply *struct{}) error {
	Task_completed_channel <- args.Body.(map[string]struct{})
	return nil
}
// Used by reducers to retrieve the results (about some keys)
func (h Mapper_handler) Get_job_full(args *Request, reply *interface{}) error {
	Job_full_request_channel <- args
	return nil
}
// Used by the reducer to check if the mapper is still alive
func (h Mapper_handler) Are_you_alive(args *Request, reply *interface{}) error {
	*reply = args.Receiver.Id == This_server.Id
	return nil
}
