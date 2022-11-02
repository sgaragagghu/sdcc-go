package mapper

import (
	. "../share"
	. "../rpc_mapper"
	. "../rpc_master"
	"fmt"
	"os"
	"io/ioutil"
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
//	"errors"

	"github.com/elliotchance/orderedmap"
)

var (
	server *Server
	stub_storage StubMapping
)

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
		InfoLoggerPtr.Println("Heartbeat sent.")
		time.Sleep(EXPIRE_TIME * SECOND / 2)
	}

}

func mapper_algorithm_clustering(properties_amount int, keys *[]string, separate_entries byte, separate_properties byte, parameters []interface{}, load []byte) (map[string]interface{}) {

	res := make(map[string]interface{})

	k := parameters[0].(int)

	u_vec := make([][]float64, k)
	for i := range u_vec {
		//u_vec[i] = make([]int, properties_amount)
		u_vec[i] = parameters[i + 1].([]float64)
	}

	reader := bytes.NewReader(load)
	buffered_read := bufio.NewReader(reader)

	for {
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

		min_index := 0
		var min float64 = -1
		for i := k - 1 ; i >= 0; i -= 1 {
			if distance := Euclidean_distance(properties_amount, u_vec[i], point); distance < min || min == -1 {
				min_index = i
				min = distance
			}
		}
		min_index_s := strconv.Itoa(min_index)
		if m, ok := res[min_index_s]; ok {
			m.(map[string]struct{})[full_s] = struct{}{}
		} else {
			*keys = append(*keys, min_index_s)
			m = make(map[string]struct{})
			m.(map[string]struct{})[full_s] = struct{}{}
			res[min_index_s] = m
		}
		//InfoLoggerPtr.Println("Element", full_s, "added to", min_index_s)
	}
	return res
}

func job_manager_goroutine(job_ptr *Job, chan_ptr *chan *Job) {

	// TODO check possible overflow
	load_ptr := Http_download(job_ptr.Resource_link, job_ptr.Begin, job_ptr.Begin + int64(math.Abs(float64((job_ptr.End - job_ptr.Begin)) * ((100 + float64(job_ptr.Margin))/100))))

	actual_begin, err := Get_actual_begin(load_ptr, job_ptr.Separate_entries)
	if err != nil { ErrorLoggerPtr.Fatal("get_actual_begin error:", err) }

	actual_end, err := Get_actual_end(load_ptr, job_ptr.Separate_entries, job_ptr.End - job_ptr.Begin)
	if err != nil { ErrorLoggerPtr.Fatal("get_actual_end error:", err) }

	//InfoLoggerPtr.Println("Actual begin:", actual_begin, "actual end:", actual_end)

	keys := make([]string, 0)

	// TODO check the error
	res, err := Call("mapper_algorithm_" + job_ptr.Algorithm, stub_storage, int(job_ptr.Properties_amount), &keys,
		job_ptr.Separate_entries, job_ptr.Separate_properties, job_ptr.Algorithm_parameters, (*load_ptr)[actual_begin:actual_end])
	if err != nil { ErrorLoggerPtr.Fatal("Error calling mapper_algorithm:", err) }
	job_ptr.Result = res.(map[string]interface {})
	job_ptr.Keys = keys
	select {
	case *chan_ptr <- job_ptr:
	default:
		ErrorLoggerPtr.Fatal("Finished job channel full.")
	}
}

func send_completed_job_goroutine(job_ptr *Job) {

	// TODO probably it is needed to use the already connection which is in place for the heartbeat

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", MASTER_IP + ":" + MASTER_PORT)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	var reply int

	err = client.Call("Master_handler.Job_mapper_completed", job_ptr, &reply)
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}
	InfoLoggerPtr.Println("Completed job sent to the master.")

}

func send_job_full_goroutine(server *Server, load *Request, log_message string) {

	// TODO probably it is needed to use the already connection which is in place for the heartbeat

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", server.Ip + ":" + server.Port)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	var reply int

	err = client.Call("Reducer_handler.Send_job_full", load, &reply)
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}
	InfoLoggerPtr.Println(log_message)
}

func prepare_and_send_job_full_goroutine(request_ptr *Request, jobs_hashmap map[string]*Job) {

	InfoLoggerPtr.Println("Preparing keys", request_ptr.Body.([]string)[1:],"full jobs of task", request_ptr.Body.([]string)[0], "for server", request_ptr.Sender.Id)

	keys_x_values := make(map[string]interface{})

	for _, v := range jobs_hashmap {
		for _, key := range request_ptr.Body.([]string)[1:] {
			if res, ok := v.Result[key]; ok {
				value, ok2 := keys_x_values[key]
				if !ok2 {
					keys_x_values[key] = res
				} else {
					for index2, value3 := range value.(map[string]struct{}) { // bad, i should pass an JOIN function instead of this for generalization.
						keys_x_values[key].(map[string]struct{})[index2] = value3
					}
				}
			}
		}
	}

	req := &Request{server, request_ptr.Sender, 0, time.Now(), keys_x_values}

	go send_job_full_goroutine(req.Receiver, req, "Sent job full " + request_ptr.Body.([]string)[0] + " to the reducer " + req.Receiver.Id)
}

func task_manager_goroutine() {

	state := IDLE
	task_hashmap := make(map[string]*list.List)
	task_finished_hashmap := orderedmap.NewOrderedMap()
	ready_event_channel := make(chan struct{}, 1000)
	job_finished_channel := make(chan *Job, 1000)
	job_finished_channel_ptr := &job_finished_channel
	next_check_task := ""

	for {
		select {
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
				select {
				case ready_event_channel <- struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("ready_event_channel is full.")
				}
			}

		case job_finished_ptr := <-*job_finished_channel_ptr:
			InfoLoggerPtr.Println("Job", job_finished_ptr.Id, "of task", job_finished_ptr.Task_id, "is finished")
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

			if len(task_hashmap) > 0 {
				select{
				case ready_event_channel <- struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("ready_event_channel is full.")
				}
			}

			// TODO POBRABILE BUG QUI
			job_light := *job_finished_ptr
			job_light.Result = nil
			go send_completed_job_goroutine(&job_light) // TODO add and manage errors

			state = IDLE

		case <-ready_event_channel:
			if state == IDLE {
				if len(task_hashmap) > 0 {
					min := -1
					task_id_int := -1
					var err error
					for task_id, _ := range task_hashmap { // TODO change the hashmap with an ordered one.
						task_id_int, err = strconv.Atoi(task_id)
						if err != nil {
							ErrorLoggerPtr.Fatal("String to integer error:", err) // TODO consider to use a integer instead of a string.
						}
						if min == -1 || min > task_id_int { min = task_id_int }
					}
					job_ptr := task_hashmap[strconv.Itoa(task_id_int)].Front().Value.(*Job)
					go job_manager_goroutine(job_ptr, job_finished_channel_ptr)
					state = BUSY
				} else { ErrorLoggerPtr.Fatal("Unexpected empty task hashmap.") }
			}
		case request_ptr := <-Job_full_request_channel:
			jobs_hashmap, ok := task_finished_hashmap.Get(request_ptr.Body.([]string)[0])
			if !ok { ErrorLoggerPtr.Fatal("Missing task") }
			go prepare_and_send_job_full_goroutine(request_ptr, jobs_hashmap.(map[string]*Job))
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
						if value.Delete { delete(job_map_ptr, key) }
					}
					if len(job_map_ptr) == 0 { task_finished_hashmap.Delete(checking_task) }
				} else { ErrorLoggerPtr.Fatal("Task is missing") }
			}
		}
	}
}

func init() {

	Job_full_request_channel = make(chan *Request, 1000)

	gob.Register([]interface{}(nil))
	gob.Register(map[string]interface{}(nil))
	gob.Register(map[string]struct{}(nil))

	stub_storage = map[string]interface{}{
		"mapper_algorithm_clustering": mapper_algorithm_clustering,
		//"funcB": funcB,
	}

	ip := GetOutboundIP().String()
	var id string

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

	if id == "" {
		ErrorLoggerPtr.Fatal("Empty ID.", err)
	}

	server = &Server{id, ip, MAPPER_PORT, time.Now(), nil, "MAPPER"}
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

