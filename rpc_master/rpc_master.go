package rpc_master

import (
	. "../share"
	json_parse "encoding/json"
	"net/http"
	"errors"
//	. "../rpc_mapper"
//	"time"
)

const (
	MASTER_PORT = "9999"
	MASTER_PORT_JSON_RPC = "8080"
)

var (
	Heartbeat_channel chan *Server
	Job_mapper_completed_channel chan *Job
	Job_reducer_completed_channel chan *Job
	Task_from_JRPC_channel chan *[]interface{}
	Result_for_JRPC_channel chan *string
	Status_ptr *Status
)


type Status struct {
	Mapper_amount int
	Reducer_amount int
}
/*
type status_json struct {
	Mapper_amount int `json:"mapper_amount"`
	Reducer_amount int `json:"reducer_amount"`
}
*/
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

// create a type to get an interface for JSONRPC
type JSONServer struct{}

func (h JSONServer) Get_status(r *http.Request, args *struct{}, reply []byte) error {
	stat := *Status_ptr
	reply, err := json_parse.Marshal(stat)
	if err != nil {
		ErrorLoggerPtr.Println("Get_status failed",  err)
	}

	return nil
}

func (h JSONServer) Send_task(r *http.Request, args *[]byte, reply bool) error {
	slice := make([]interface{}, 2)
	slice[0] = make(chan bool, 1)
	slice[1] = args

	select {
	case Task_from_JRPC_channel <- &slice:
		select {
		case reply = <-slice[0].(chan bool):
		}
	default:
		WarningLoggerPtr.Println("JRPC task channel is full")
		reply = false
		return errors.New("JRPC task channel is full")
	}

	return nil
}


func (h JSONServer) Get_results(r *http.Request, args *struct{}, reply []byte) error {
	results := ""

	for loop := true; loop; {
		select {
		case result := <-Result_for_JRPC_channel:
			results += result
		default:
			loop = false
		}
	}
	reply, err := json_parse.Marshal(results)
	if err != nil {
		ErrorLoggerPtr.Println("Get_results failed",  err)
	}

	return nil
}
