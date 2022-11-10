package rpc_master

import (
	. "../share"
//	json_parse "encoding/json"
	"net/http"
	"errors"
//	. "../rpc_mapper"
//	"time"


	"github.com/elliotchance/orderedmap"
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

type Task struct {
	Id int32 `json:"id, omitempty"`
	Origin_id int32 `json:"origin_id, omitempty"`
	Send_time int64 `json:"send_time, omitempty"`
	Resource_link string `json:"resource_link, omitempty"`
	Mappers_amount int32 `json:"mappers_amount, omitempty"`
	Margin int8 `json:"margin, omitempty"`
	Separate_entries byte `json:"separate_entries, omitempty"`
	Separate_properties byte `json:"separate_properties, omitempty"`
	Properties_amount int8 `json:"properties_amount, omitempty"`
	Initialization_algorithm string `json:"initialization_algorithm, omitempty"`
	Map_algorithm string `json:"map_algorithm, omitempty"`
	Map_algorithm_parameters interface{} `json:"map_algorithm_parameters, omitempty"`
//	Shuffle_algorithm string
//	Order_algorithm string
	Reducers_amount int32 `json:"reducers_amount, omitempty"`
	Reduce_algorithm string `json:"reduce_algorithm, omitempty"`
	Reduce_algorithm_parameters interface{} `json:"reduce_algorithm_parameters, omitempty"`
	Iteration_algorithm string `json:"iteration_algorithm, omitempty"`
	Iteration_algorithm_parameters interface{} `json:"iteration_algorithm_parameters, omitempty"`
	Keys_x_servers *orderedmap.OrderedMap `json:"keys_x_servers, omitempty"`
	Keys_x_servers_version int8 `json:"keys_x_servers, omitempty"`
	Jobs map[string]*Job `json:"jobs, omitempty"`
	Jobs_done map[string]*Job `json:"mapper_amount, omitempty"`
}

type Status struct {
	Mapper_amount int `json:"mapper_amount"`
	Reducer_amount int `json:"reducer_amount"`
}

type Results_type struct {
	Results string `json:"results"`
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

func (h JSONServer) Get_status(r *http.Request, _ *struct{}, reply *Status) error {
	stat := *Status_ptr
	reply = &stat
	/*
	full_bytes, err := json_parse.Marshal(stat)
	full_string := string(full_bytes)
	reply = full_string
	if err != nil {
		ErrorLoggerPtr.Println("Get_status failed",  err)
	}
*/
	return nil
}

func (h JSONServer) Send_task(r *http.Request, args *Task, reply *bool) error {
	InfoLoggerPtr.Println("ciao")
	slice := make([]interface{}, 2)
	slice[0] = make(chan bool, 1)
	slice[1] = args
	reply_val := false

	select {
	case Task_from_JRPC_channel <- &slice:
		select {
		case reply_val = <-slice[0].(chan bool):
			reply = &reply_val
		}
	default:
		WarningLoggerPtr.Println("JRPC task channel is full")
		return errors.New("JRPC task channel is full")
	}

	return nil
}


func (h JSONServer) Get_results(r *http.Request, _ *struct{}, reply *Results_type) error {
	results := ""


	for loop := true; loop; {
		select {
		case result := <-Result_for_JRPC_channel:
			results += *result
		default:
			loop = false
		}
	}
//	full_string, err := json_parse.Marshal(results)
	reply = &Results_type{results}

//	if err != nil {
//		ErrorLoggerPtr.Println("Get_results failed",  err)
//	}

	return nil
}

