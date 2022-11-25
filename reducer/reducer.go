package reducer

import (
	. "../share"
	. "../rpc_reducer"
	. "../rpc_master"
	//. "../rpc_mapper"
	"fmt"
	//"os"
	//"io/ioutil"
	"strconv"
	"time"
	"net/rpc"
	"crypto/sha256"
	"encoding/gob"
	"net"
	"reflect"
	"container/list"
	"bytes"
	"bufio"
	//"io"
	//"math"
	//"errors"


	"github.com/elliotchance/orderedmap"
)

var (
	server *Server
	master *Server
	// data structure for calling functions by its name in a string
	stub_storage StubMapping
)

// Sending the heartbeat every (EXPIRE_TIME / 2) seconds
func heartbeat_goroutine(client *rpc.Client) {

	var (
		reply	int
		err	error
	)

	for {
		server.Last_heartbeat = time.Now()
		err = client.Call("Master_handler.Send_heartbeat", server, &reply)
		if err != nil {
			ErrorLoggerPtr.Fatal("Heartbeat error:", err)
		}
		//InfoLoggerPtr.Println("Heartbeat sent.")
		time.Sleep(EXPIRE_TIME * SECOND / 2)
	}

}

// Reducer "custom" method
func reducer_algorithm_clustering(properties_amount int, keys []string, keys_x_values map[string]map[string]struct{},
		separate_properties byte, separate_entries byte, parameters []interface{}) (map[string]interface{}) {
	// res map[key]centroid
	res := make(map[string]interface{})

	// keys_x_values is map[key]map[point] empty struct
	for index, value := range keys_x_values {
		mean := make([]float64, properties_amount)
		var n_samples float64 = 1
		for string_point, _ := range value {
			reader := bytes.NewReader([]byte(string_point))
			buffered_read := bufio.NewReader(reader)
			point := make([]float64, properties_amount)
			// points are encoded in a string (which is the key of the internal map)
			Parser_simple(&point, buffered_read, separate_properties, separate_entries)
			// online mean to avoid overflow
			mean = Welford_one_pass(mean, point, n_samples)
			n_samples += 1
		}
		res[index] = mean
	}
	return res
}

// method that takes care of completing the job
func job_manager_goroutine(job_ptr *Job, chan_ptr *chan *Job) {
	// request_map is orderedmap[server]request struct (which contains the slice of needed keys from that specific server)
	requests_map := orderedmap.NewOrderedMap()
	// keys_x_values is map[key]map[point] empty struct
	keys_x_values := make(map[string]map[string]struct{})

	// basically inverting keys_x_servers to servers_x_keys
	for i, v_map := range job_ptr.Keys_x_servers {
		for index, v := range v_map {
			value_ptr, ok := requests_map.Get(index)
			if !ok {
				keys := make([]string, 1)
				keys[0] = job_ptr.Task_id
				value_ptr = &Request{server, v, 0, time.Now(), keys}
				requests_map.Set(index, value_ptr)

			}
			value_ptr.(*Request).Body = append(value_ptr.(*Request).Body.([]string), i)
		}


	}
	// for each server, asking the respective keys
	for el := requests_map.Front(); el != nil; el = el.Next() {
		req := el.Value.(*Request)
		req.Time = time.Now()
		go Rpc_request_goroutine(req.Receiver, req, "Mapper_handler.Get_job_full",
			"Requesting keys " + fmt.Sprint(req.Body.([]string)[1:]) + " of task " + req.Body.([]string)[0] + " from server " + req.Receiver.Id,
			3, EXPIRE_TIME, true)
	}

	alg_join, ok := job_ptr.Algorithm["join"]
	if !ok { ErrorLoggerPtr.Println("Missing algorithm") }
	alg_join_par, ok := job_ptr.Algorithm_parameters["join"]
	if !ok { ErrorLoggerPtr.Println("Missing algorithm parameter") }

	// waiting the answer from each request
	for loop := true; loop; {
		select {
			case job_full := <-Job_full_channel: // type: Request
			requests_map.Delete(job_full.Sender.Id)
			// combining the result with the result we received previously
			for i, v := range job_full.Body.(map[string]interface{}) {
				_, ok := keys_x_values[i]
				if !ok {
					keys_x_values[i] = v.(map[string]struct{})
				} else {
					_, err := Call("Join_algorithm_" + alg_join, stub_storage, keys_x_values[i], v, alg_join_par)
					if err != nil { ErrorLoggerPtr.Fatal("Error calling mapper_algorithm:", err) }
				}
			}
			// exit point
			if requests_map.Len() == 0 { loop = false }
		// checking if the server we are waiting the data is still working
		case <-time.After(3 * EXPIRE_TIME * SECOND):
			req := requests_map.Front().Value.(*Request)
			{
				reply := Rpc_request_goroutine(req.Receiver, req, "Mapper_handler.Are_you_alive",
					"Waiting time expired, checking if the mapper " + req.Receiver.Id + " is alive.",
					3, EXPIRE_TIME, true)

				if reply == nil || reply.(bool) == false {
					ErrorLoggerPtr.Fatal("Mapper", req.Receiver.Id, "doesn't answer.")
				} else {
					InfoLoggerPtr.Println("Mapper", req.Receiver.Id, "still alive.")
				}
			}
		}
	}
	keys := make([]string, 1)


	alg, ok := job_ptr.Algorithm["reduce"]
	if !ok { ErrorLoggerPtr.Println("Missing algorithm") }
	alg_par, ok := job_ptr.Algorithm_parameters["reduce"]
	if !ok { ErrorLoggerPtr.Println("Missing algorithm parameters") }
	// calling the reducer function
	res, err := Call("reducer_algorithm_" + alg, stub_storage, int(job_ptr.Properties_amount), keys,
		keys_x_values, job_ptr.Separate_properties, job_ptr.Separate_entries, alg_par)
	if err != nil { ErrorLoggerPtr.Fatal("Error calling reducer_algorithm:", err) }
	job_ptr.Result = res.(map[string]interface {})
	job_ptr.Keys = keys
	select {
	// notify the sceduler that the join has been completed
	case *chan_ptr <- job_ptr:
	default:
		ErrorLoggerPtr.Fatal("Finished job channel full.")
	}
}

func task_manager_goroutine() {

	state := IDLE
	// map[task]list of jobs
	task_hashmap := make(map[string]*list.List)
	//task_finished_hashmap := orderedmap.NewOrderedMap()
	ready_event_channel := make(chan struct{}, 1000)
	job_finished_channel := make(chan *Job, 1000)
	job_finished_channel_ptr := &job_finished_channel
	//next_check_task := ""

	for {
		select {
		// new job received
		case job_ptr := <-Job_reducer_channel:
			InfoLoggerPtr.Println("Received Task", job_ptr.Task_id, "job", job_ptr.Id, ".")

			{
				job_list_ptr, ok := task_hashmap[job_ptr.Task_id]
				if !ok {
					job_list_ptr = new(list.List)
					task_hashmap[job_ptr.Task_id] = job_list_ptr
				}
					job_list_ptr.PushBack(job_ptr)
			}

			if state == IDLE {
				// run the next job, if any
				select {
				case ready_event_channel <- struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("ready_event_channel is full.")
				}
			}
		// job has been completed, just updating the data structures
		case job_finished_ptr := <-*job_finished_channel_ptr:
			InfoLoggerPtr.Println("Job", job_finished_ptr.Id, "of task", job_finished_ptr.Task_id, "is finished.")
			// moving the job to the variable containing the completed ones
			if job_list_ptr, ok := task_hashmap[job_finished_ptr.Task_id]; ok {
				job_list_ptr.Remove(job_list_ptr.Front())
				if job_list_ptr.Len() == 0 { delete (task_hashmap, job_finished_ptr.Task_id) }
			} else { ErrorLoggerPtr.Fatal("Finished job not found!") }
			/*
			{
				job_map, ok := task_finished_hashmap.Get(job_finished_ptr.Task_id)
				if !ok {
					job_map = make(map[string]*Job)
					task_finished_hashmap.Set(job_finished_ptr.Task_id, job_map)
				}
				job_map.(map[string]*Job)[job_finished_ptr.Id] = job_finished_ptr
			}
			*/


			// run the next job, if any
			if len(task_hashmap) > 0 {
				select{
				case ready_event_channel <- struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("ready_event_channel is full.")
				}
			}

			go Rpc_job_goroutine(master, job_finished_ptr, "Master_handler.Job_reducer_completed",
				"Sent completed job " + job_finished_ptr.Id + " of task " + job_finished_ptr.Task_id,
				3, EXPIRE_TIME, true)

			state = IDLE
		// run the next job, if any
		case <-ready_event_channel:
			if state == IDLE {
				if len(task_hashmap) > 0 {
					// Serving the oldest task's job
					min := -1
					task_id_int := -1
					var err error
					for task_id, _ := range task_hashmap {
						task_id_int, err = strconv.Atoi(task_id)
						if err != nil {
							ErrorLoggerPtr.Fatal("String to integer error:", err)
						}
						if min == -1 || min > task_id_int { min = task_id_int }
					}
					job_ptr := task_hashmap[strconv.Itoa(task_id_int)].Front().Value.(*Job)
					go job_manager_goroutine(job_ptr, job_finished_channel_ptr)
					state = BUSY
				} else { ErrorLoggerPtr.Fatal("Unexpected empty task hashmap.") }
			}
		/*case <-time.After(10 * SECOND):
			if task_finished_hashmap.Front() != nil {
				if next_check_task == "" { next_check_task = task_finished_hashmap.Front().Key.(string) }
				if el := task_finished_hashmap.GetElement(next_check_task); el != nil {
					job_map_ptr := el.Value.(map[string]*Job)
					checking_task := next_check_task
					if el = el.Next(); el != nil {
						next_check_task = el.Key.(string)
					} else if el = task_finished_hashmap.Front(); el != nil {
						next_check_task = el.Key.(string)
					} else {
						next_check_task = ""
					}
					for key, value := range job_map_ptr {
						if value.Delete { delete(job_map_ptr, key) }
					}
					if len(job_map_ptr) == 0 { task_finished_hashmap.Delete(checking_task) }
				} else { ErrorLoggerPtr.Fatal("Task is missing") }
			}
		*/
		}
	}
}

func init() {
	// registering types for interface{}
	gob.Register([]interface{}(nil))
	gob.Register(map[string]interface{}(nil))
	gob.Register(map[string]struct{}(nil))
	// data structure for calling functions by its name in a string
	stub_storage = map[string]interface{}{
		"reducer_algorithm_clustering": reducer_algorithm_clustering,
		"Join_algorithm_clustering": Join_algorithm_clustering,
	}
	// geting our IP
	ip := GetOutboundIP().String()
	var id string
	// getting reducer ID
	id = ip + REDUCER_PORT + APP_ID + strconv.FormatInt(time.Now().Unix(), 10)
	id = fmt.Sprintf("%x", sha256.Sum256([]byte(id)))

	if id == "" {
		ErrorLoggerPtr.Fatal("Empty ID."/*, err*/)
	}

	server = &Server{id, ip, REDUCER_PORT, time.Now(), "REDUCER"}
	master = &Server{"", MASTER_IP, MASTER_PORT, time.Now(), "MASTER"}
}

func Reducer_main() {
	var client *rpc.Client
	var err error
	for {
		// connect to server via rpc tcp
		client, err = rpc.Dial("tcp", MASTER_IP + ":" + MASTER_PORT)
		defer client.Close()
		if err != nil {
			WarningLoggerPtr.Println(err)
			time.Sleep( (EXPIRE_TIME / 2) * SECOND )
		} else {
			break
		}
	}

	// creating channel for communicating the task
	// to the goroutine task manager
	Job_reducer_channel = make(chan *Job, 1000)
	Job_full_channel = make(chan *Request, 1000)
	//go task_goroutine()

	go task_manager_goroutine()
	go heartbeat_goroutine(client)

	reducer_handler := new(Reducer_handler)

	// register Reducer_handler as RPC interface
	rpc.Register(reducer_handler)

	// service address of server
	service := ":" + REDUCER_PORT

	// create tcp address
	tcpAddr, err := net.ResolveTCPAddr("tcp", service)
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	// tcp network listener
	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	for {
		// handle tcp reducer connections
		conn, err := listener.Accept()
		if err != nil {
			WarningLoggerPtr.Println("listener accept error:", err)
		}

		// print connection info
		InfoLoggerPtr.Println("received message", reflect.TypeOf(conn), conn)

		// handle reducer connections via rpc
		go rpc.ServeConn(conn)
	}
}
