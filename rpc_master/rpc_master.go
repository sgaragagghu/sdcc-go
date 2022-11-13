package rpc_master

import (
	. "../share"
//	json_parse "encoding/json"
	"net/http"
	"errors"
//	"reflect"
//	"strings"
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
	Result_for_JRPC_channel chan *Task_result
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

type Task_json struct {
	Resource_link string `json:"resource_link"`
	Mappers_amount int32 `json:"mappers_amount"`
	Margin int8 `json:"margin"`
	Separate_entries string `json:"separate_entries"`
	Separate_properties string `json:"separate_properties"`
	Properties_amount int8 `json:"properties_amount"`
	Initialization_algorithm string `json:"initialization_algorithm"`
	Map_algorithm string `json:"map_algorithm"`
	Map_algorithm_parameters interface{} `json:"map_algorithm_parameters"`
	Reducers_amount int32 `json:"reducers_amount"`
	Reduce_algorithm string `json:"reduce_algorithm"`
	Reduce_algorithm_parameters interface{} `json:"reduce_algorithm_parameters"`
	Iteration_algorithm string `json:"iteration_algorithm"`
	Iteration_algorithm_parameters interface{} `json:"iteration_algorithm_parameters"`
}

type Status struct {
	Mapper_amount int `json:"mapper_amount"`
	Reducer_amount int `json:"reducer_amount"`
}

type Task_result struct {
	Id int32 `json:"id"`
	Origin_id int32 `json:"origin_id"`
	Result string `json:"result"`
}

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

func (h JSONServer) Send_task(r *http.Request, args *Task_json, reply *bool) error {

	task_ptr := &Task{
		Id:-1,
		Origin_id:-1,
		Send_time:0,
		Resource_link:args.Resource_link,
		Mappers_amount:args.Mappers_amount,
		Margin:args.Margin,
		Separate_entries:([]byte(args.Separate_entries))[0],
		Separate_properties:([]byte(args.Separate_properties))[0],
		Properties_amount:args.Properties_amount,
		Initialization_algorithm:args.Initialization_algorithm,
		Map_algorithm:args.Map_algorithm,
		Map_algorithm_parameters:args.Map_algorithm_parameters,
		Reducers_amount:args.Reducers_amount,
		Reduce_algorithm:args.Reduce_algorithm,
		Reduce_algorithm_parameters:args.Reduce_algorithm_parameters,
		Iteration_algorithm:args.Iteration_algorithm,
		Iteration_algorithm_parameters:args.Iteration_algorithm_parameters,
		Keys_x_servers:orderedmap.NewOrderedMap(),
		Keys_x_servers_version:0,
		Jobs:make(map[string]*Job),
		Jobs_done:make(map[string]*Job),
	}

	slice := make([]interface{}, 2)
	slice[0] = make(chan bool, 1)
	slice[1] = task_ptr
	reply_value := false
	var err error = nil

	select {
	case Task_from_JRPC_channel <- &slice:
		select {
		case reply_value = <-slice[0].(chan bool):
		}
	default:
		WarningLoggerPtr.Println("JRPC task channel is full")
		err = errors.New("JRPC task channel is full")
	}
	*reply = reply_value
	return err
}


func (h JSONServer) Get_results(r *http.Request, _ *struct{}, reply *[]Task_result) error {


	for loop := true; loop; {
		select {
		case task_result_ptr := <-Result_for_JRPC_channel:
			*reply = append(*reply, *task_result_ptr)
		default:
			loop = false
		}
	}

	return nil
}


/*
func (h JSONServer) Get_logs(r *http.Request, _ *struct{}, reply *string) error {
	results := ""


	for loop := true; loop; {
		select {
		case result := <-Result_for_JRPC_channel:
			results += *result
		default:
			loop = false
		}
	}

	*reply = results

	return nil
}
*/
