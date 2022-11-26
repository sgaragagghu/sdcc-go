package rpc_reducer

import (
	. "../share"
//	. "../rpc_mapper"
//	"container/list"
)

const REDUCER_PORT = "6665"

var (
	This_server *Server
	Job_reducer_channel chan *Job
	Job_full_channel chan *Request
)

// create a type to get an interface
type Reducer_handler int
// Used by the master to send jobs to the reducer
func (h Reducer_handler) Send_job(args *Job, reply *interface{}) error {
	server_id := args.Server_id
	if This_server.Id != server_id {
		*reply = false
		return nil
	}
	Job_reducer_channel <- args
	*reply = true
	return nil
}
// Used by mappers to send requested key'skey's results
func (h Reducer_handler) Send_job_full(args *Request, reply *interface{}) error {
	Job_full_channel <- args
	return nil
}

