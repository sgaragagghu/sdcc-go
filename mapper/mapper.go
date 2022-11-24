package mapper

import (
	. "../share"
	. "../rpc_mapper"
	. "../rpc_master"
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
	"io"
	"math"
	//"errors"

	"github.com/elliotchance/orderedmap"
)

var (
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
		This_server.Last_heartbeat = time.Now()
		err = client.Call("Master_handler.Send_heartbeat", This_server, &reply)
		if err != nil {
			ErrorLoggerPtr.Fatal("Heartbeat error:", err)
		}
		InfoLoggerPtr.Println("Heartbeat sent.")
		time.Sleep(EXPIRE_TIME * SECOND / 2)
	}

}

// Mapper "custom" method
func mapper_algorithm_clustering(properties_amount int, keys *[]string, separate_entries byte, separate_properties byte, parameters []interface{}, load []byte) (map[string]interface{}) {
	// result, the interface will be actually another map which will contain every point assigned to that centroid
	// so: map[centroid][point]empty struct
	res := make(map[string]interface{})
	// number of clusters
	k := parameters[0].(int)
	// array of cluester's centroids
	u_vec := make([][]float64, k)
	// filling the array with the centroids saved in the vector variable parameters
	for i := range u_vec {
		//u_vec[i] = make([]int, properties_amount)
		u_vec[i] = parameters[i + 1].([]float64)
	}
	// getting a buffered reader, load is the variable which contains the raw data
	reader := bytes.NewReader(load)
	buffered_read := bufio.NewReader(reader)

	for {
		// parsing one entry each time, point is containing the result
		point := make([]float64, properties_amount)
		full_s, err := Parser_simple(&point, buffered_read, separate_properties, separate_entries)
		//InfoLoggerPtr.Println("string", full_s, "point", point, "error", err)
		if err != nil {
			if err == io.EOF {
				break
			} else {
				ErrorLoggerPtr.Fatal(err)
			}
		}
		// seaching the nearest centroid
		min_index := 0
		var min float64 = -1
		for i := k - 1 ; i >= 0; i -= 1 {
			if distance := Euclidean_distance(properties_amount, u_vec[i], point); distance < min || min == -1 {
				min_index = i
				min = distance
			}
		}

		min_index_s := strconv.Itoa(min_index)
		// combining
		if m, ok := res[min_index_s]; ok {
			m.(map[string]struct{})[full_s] = struct{}{}
		} else {
			// if a point is assigned to this centroid for the first time, then we append the centroid's key to the slice variable key
			*keys = append(*keys, min_index_s)
			m = make(map[string]struct{})
			m.(map[string]struct{})[full_s] = struct{}{}
			res[min_index_s] = m
		}
		//InfoLoggerPtr.Println("Element", full_s, "added to", min_index_s)
	}
	return res
}

// method that takes care of completing the job
func job_manager_goroutine(job_ptr *Job, chan_ptr *chan *Job) {

	if math.Abs(float64((job_ptr.End - job_ptr.Begin)) * ((100 + float64(job_ptr.Margin))/100)) > math.MaxInt64 { ErrorLoggerPtr.Println("Overflow!!") }
	// downloading the assigned slice of the file, plus a margin (a percentage of the total length of the slice)
	// the margin is needed because probably we do not know where the last entry ends
	load_ptr := Http_download(job_ptr.Resource_link, job_ptr.Begin, job_ptr.Begin + int64(math.Abs(float64((job_ptr.End - job_ptr.Begin)) * ((100 + float64(job_ptr.Margin))/100))))

	// finding the begin of the first entry
	actual_begin, err := Get_actual_begin(load_ptr, job_ptr.Separate_entries)
	if err != nil { ErrorLoggerPtr.Fatal("get_actual_begin error:", err) }
	// finding the end of the last entry
	actual_end, err := Get_actual_end(load_ptr, job_ptr.Separate_entries, job_ptr.End - job_ptr.Begin)
	if err != nil { ErrorLoggerPtr.Fatal("get_actual_end error:", err) }

	//InfoLoggerPtr.Println("Actual begin:", actual_begin, "actual end:", actual_end)

	keys := make([]string, 0)

	alg, ok := job_ptr.Algorithm["map"]
	if !ok { ErrorLoggerPtr.Println("Missing algorithm") }
	alg_par, ok := job_ptr.Algorithm_parameters["map"]
	if !ok { ErrorLoggerPtr.Println("Missing algorithm parameter") }
	// calling the map method
	res, err := Call("mapper_algorithm_" + alg, stub_storage, int(job_ptr.Properties_amount), &keys,
		job_ptr.Separate_entries, job_ptr.Separate_properties, alg_par, (*load_ptr)[actual_begin:actual_end])
	if err != nil { ErrorLoggerPtr.Fatal("Error calling mapper_algorithm:", err) }
	job_ptr.Result = res.(map[string]interface {})
	job_ptr.Keys = keys
	// notifying the scheduler that the job is done
	select {
	case *chan_ptr <- job_ptr:
	default:
		ErrorLoggerPtr.Fatal("Finished job channel full.")
	}
}

// sending requested jobs to the reducer
func prepare_and_send_job_full_goroutine(request_ptr *Request, jobs_hashmap map[string]*Job) {

	InfoLoggerPtr.Println("Preparing keys", request_ptr.Body.([]string)[1:],"full jobs of task", request_ptr.Body.([]string)[0], "for server", request_ptr.Sender.Id)

	keys_x_values := make(map[string]interface{})

	for _, v := range jobs_hashmap {

		alg, ok := v.Algorithm["join"]
		if !ok { ErrorLoggerPtr.Println("Missing algorithm") }
		alg_par, ok := v.Algorithm_parameters["join"]
		if !ok { ErrorLoggerPtr.Println("Missing algorithm parameter") }
		// combinig jobs results retrieving the requested keys
		for _, key := range request_ptr.Body.([]string)[1:] {
			if res, ok := v.Result[key]; ok {

				value, ok2 := keys_x_values[key]
				if !ok2 {
					keys_x_values[key] = res
				} else {
					_, err := Call("Join_algorithm_" + alg, stub_storage, keys_x_values[key], value, alg_par)
					if err != nil { ErrorLoggerPtr.Fatal("Error calling mapper_algorithm:", err) }
				}
			}
		}
	}
	// sending back the answer
	req := &Request{This_server, request_ptr.Sender, 0, time.Now(), keys_x_values}
	go Rpc_request_goroutine(req.Receiver, req, "Reducer_handler.Send_job_full",
		"Sent job full " + request_ptr.Body.([]string)[0] + " to the reducer " + req.Receiver.Id,
		3, EXPIRE_TIME, true)

}

func task_manager_goroutine() {

	state := IDLE
	// map[task]list of jobs
	task_hashmap := make(map[string]*list.List)
	// orderedmap[task]list of jobs
	task_finished_hashmap := orderedmap.NewOrderedMap()
	ready_event_channel := make(chan struct{}, 1000)
	job_finished_channel := make(chan *Job, 1000)
	job_finished_channel_ptr := &job_finished_channel
	// there is a cleaning routine and here it saves the next task to be checked for deletion
	next_check_task := ""
	// task ready to be deleted
	to_delete_tasks := make(map[string]string)

	for {
		select {
		// new job received
		case job_ptr := <-Job_channel:
			InfoLoggerPtr.Println("Received Task", job_ptr.Task_id, "job", job_ptr.Id)

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
			InfoLoggerPtr.Println("Job", job_finished_ptr.Id, "of task", job_finished_ptr.Task_id, "is finished")
			// moving the job to the variable containing the completed ones
			if job_list_ptr, ok := task_hashmap[job_finished_ptr.Task_id]; ok {
				job_list_ptr.Remove(job_list_ptr.Front())
				if job_list_ptr.Len() == 0 { delete (task_hashmap, job_finished_ptr.Task_id) }
			} else { ErrorLoggerPtr.Fatal("Finished job not found!") }

			{
				job_map, ok := task_finished_hashmap.Get(job_finished_ptr.Task_id)
				if !ok {
					job_map = make(map[string]*Job)
					task_finished_hashmap.Set(job_finished_ptr.Task_id, job_map)
				}
				job_map.(map[string]*Job)[job_finished_ptr.Id] = job_finished_ptr
			}

			// run the next job, if any
			if len(task_hashmap) > 0 {
				select{
				case ready_event_channel <- struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("ready_event_channel is full.")
				}
			}

			job_light := *job_finished_ptr
			job_light.Result = nil

			go Rpc_job_goroutine(master, &job_light, "Master_handler.Job_mapper_completed", "Completed job sent to the master.",
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
		// reducer rquested some keys
		case request_ptr := <-Job_full_request_channel:
			jobs_hashmap, ok := task_finished_hashmap.Get(request_ptr.Body.([]string)[0])
			if !ok { ErrorLoggerPtr.Fatal("Missing task") }
			go prepare_and_send_job_full_goroutine(request_ptr, jobs_hashmap.(map[string]*Job))
		// a task has been fully completed and elements da get deleted
		case map_completed_task := <-Task_completed_channel:
			for task_completed_id, _ := range map_completed_task {
				to_delete_tasks[task_completed_id] = task_completed_id
			}
		// slowly deleting old completed tasks
		case <-time.After(10 * SECOND):
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
						//if value.Delete { delete(job_map_ptr, key) }
						if _, ok := to_delete_tasks[value.Task_id]; ok { delete(job_map_ptr, key) }
					}
					if len(job_map_ptr) == 0 { task_finished_hashmap.Delete(checking_task) }
				} else { ErrorLoggerPtr.Fatal("Task is missing") }
			}
		}
	}
}

func init() {

	Job_full_request_channel = make(chan *Request, 1000)
	// registering types for interface{}
	gob.Register([]interface{}(nil))
	gob.Register(map[string]interface{}(nil))
	gob.Register(map[string]struct{}(nil))

	// data structure for calling functions by its name in a string
	stub_storage = map[string]interface{}{
		"mapper_algorithm_clustering": mapper_algorithm_clustering,
		"Join_algorithm_clustering": Join_algorithm_clustering,
	}

	// geting our IP
	ip := GetOutboundIP().String()
	var id string

	// getting mapper ID
	id = ip + MAPPER_PORT + APP_ID + strconv.FormatInt(time.Now().Unix(), 10)
	id = fmt.Sprintf("%x", sha256.Sum256([]byte(id)))
/*
	bytes, err := ioutil.ReadFile("./ID")
	if string(bytes) == "" || err != nil {

		id = ip + MAPPER_PORT + APP_ID + strconv.FormatInt(time.Now().Unix(), 10)
		id = fmt.Sprintf("%x", sha256.Sum256([]byte(id)))

		file, err := os.Create("./ID")
		if err != nil {
			ErrorLoggerPtr.Fatal("Cannot create ID file.", err)
		}
		file.WriteString(id)
		file.Close()
	} else {
		id = string(bytes)
	}
*/
	if id == "" {
		ErrorLoggerPtr.Fatal("Empty ID."/*, err*/)
	}

	This_server = &Server{id, ip, MAPPER_PORT, time.Now(), "MAPPER"}
	master = &Server{"", MASTER_IP, MASTER_PORT, time.Now(), "MASTER"}
}

func Mapper_main() {

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", MASTER_IP + ":" + MASTER_PORT)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	// creating channel for communicating the task
	// to the goroutine task manager
	Job_channel = make(chan *Job, 1000)

	Task_completed_channel = make(chan map[string]struct{}, 1000)

	//go task_goroutine()

	go task_manager_goroutine()
	go heartbeat_goroutine(client)

	mapper_handler := new(Mapper_handler)

	// register Mapper_handler as RPC interface
	rpc.Register(mapper_handler)

	// service address of server
	service := ":" + MAPPER_PORT

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
		// handle tcp mapper connections
		conn, err := listener.Accept()
		if err != nil {
			WarningLoggerPtr.Println("listener accept error:", err)
		}

		// print connection info
		InfoLoggerPtr.Println("received message", reflect.TypeOf(conn), conn)

		// handle mapper connections via rpc
		go rpc.ServeConn(conn)
	}
}
