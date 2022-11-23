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
	Id string // task id
	Origin_id string // task id of the first iteration
	Send_time int64 // send time
	Resource_link string // it is supposed that we are using an http resource link
	Mappers_amount int32 // maximum amount of mappers to be used
	Margin int8 // mapper doesn't download exactly its assigned slice but a bit more, it is explained in the mapper source code
	Separate_entries byte // (parser) character placed between entries
	Separate_properties byte // (parser) character placed between properties
	Properties_amount int8 // amount of properties for each entries (eg. each cartesian plane's point has 2 properties)
	Initialization_algorithm string // name of the initialization algorithm
	Map_algorithm string // name of the map algorithm
	Map_algorithm_parameters interface{}
	Reducers_amount int32 // maximum amount of reducers to be used
	Reduce_algorithm string // name of the reduce algorithm
	Reduce_algorithm_parameters interface{}
	Join_algorithm string // name of the join algorithm
	Join_algorithm_parameters interface{}
	Iteration_algorithm string // name of the iteration algorithm
	Iteration_algorithm_parameters interface{}
	Keys_x_servers *orderedmap.OrderedMap // tracking where are located the results of such keys
	Keys_x_servers_version int8 // if a mapper crashes then probably we have to modify the previous variable and then increment this
	Jobs map[string]*Job // list of to-do jobs
	Jobs_done map[string]*Job // list of completed jobs
}

// struct that is "json" friendly
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
	Join_algorithm string `json:"join_algorithm"`
	Join_algorithm_parameters interface{} `json:"join_algorithm_parameters"`
	Iteration_algorithm string `json:"iteration_algorithm"`
	Iteration_algorithm_parameters interface{} `json:"iteration_algorithm_parameters"`
}

type Status struct {
	Mapper_amount int `json:"mapper_amount"`
	Reducer_amount int `json:"reducer_amount"`
}

type Task_result struct {
	Id string `json:"id"`
	Origin_id string `json:"origin_id"`
	Result string `json:"result"`
}

// create a type to get an interface
type Master_handler int
// Used by mappers and reducers to send heartbeats
func (h Master_handler) Send_heartbeat(args *Server, reply *int) error {
	Heartbeat_channel <- args
	return nil
}
// Used by mappers to notify (keys included) that the job has been completed
func (h Master_handler) Job_mapper_completed(args *Job, reply *int) error {
	Job_mapper_completed_channel <- args
	return nil
}
// Used by the reducer to notify (result included) that the job has been completed
func (h Master_handler) Job_reducer_completed(args *Job, reply *int) error {
	Job_reducer_completed_channel <- args
	return nil
}
// create a type to get an interface for JSONRPC
type JSONServer struct{}
// Used by user or frontend to get the information about the status of the system (task, mapper and reducer amount)
func (h JSONServer) Get_status(r *http.Request, _ *struct{}, reply *Status) error {
	stat := *Status_ptr
	reply = &stat

	/*
	full_bytes, err := json_parse.Marshal(stat)
	full_string := string(full_bytes)
	reply = full_string
	if err != nil {
		ErrorLoggerPtr.Println("Get_status failed",  err)
	}*/

	return nil
}
// Used by the user or frontend to send a new task
func (h JSONServer) Send_task(r *http.Request, args *Task_json, reply *bool) error {

	task_ptr := &Task{
		Id:"-1",
		Origin_id:"-1",
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
		Join_algorithm:args.Join_algorithm,
		Join_algorithm_parameters:args.Join_algorithm_parameters,
		Iteration_algorithm:args.Iteration_algorithm,
		Iteration_algorithm_parameters:args.Iteration_algorithm_parameters,
		Keys_x_servers:orderedmap.NewOrderedMap(),
		Keys_x_servers_version:0,
		Jobs:make(map[string]*Job),
		Jobs_done:make(map[string]*Job),
	}

	slice := make([]interface{}, 2)
	slice[0] = make(chan error, 1)
	slice[1] = task_ptr
	reply_value := false
	var err error = nil

	select {
	case Task_from_JRPC_channel <- &slice:
		select {
		case err = <-slice[0].(chan error):
		}
	default:
		WarningLoggerPtr.Println("JRPC task channel is full")
		err = errors.New("JRPC task channel is full")
	}
	if err == nil { reply_value = true }
	*reply = reply_value
	return err
}

// Used by the user of frontend to get the latest results
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
