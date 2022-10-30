package reducer

import (
	. "../share"
	. "../rpc_reducer"
	. "../rpc_master"
	. "../rpc_mapper"
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
//	"io"
//	"math"
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

func reducer_algorithm_clustering(properties_amount int, keys []string, keys_x_values map[string]map[string]struct{},
		separate_properties byte, parameters []interface{}) (map[string]interface{}) {

	res := make(map[string]interface{})
/*
	k := parameters[0].(int)

	u_vec := make([][]float64, k)

	for i := range u_vec {
		//u_vec[i] = make([]int, properties_amount)
		u_vec[i] = parameters[i + 1].([]float64)
	}
*/

	for index, value := range keys_x_values {
		mean := make([]float64, properties_amount)
		var n_samples float64 = 1
		for string_point, _ := range value {
				reader := bytes.NewReader([]byte(string_point))
				buffered_read := bufio.NewReader(reader)
				point := make([]float64, properties_amount)
				j := 1
				s := ""
			for char, err := buffered_read.ReadByte(); err == nil; char, err = buffered_read.ReadByte() {
				//InfoLoggerPtr.Println(string(char))
				if char == separate_properties {
					if j <= properties_amount {
						point[j - 1], _ = strconv.ParseFloat(s, 64) // TODO check the error
						//full_s += string(separate_properties) + s
						s = ""
						break
					} else { ErrorLoggerPtr.Fatal("Parsing failed") }
				} else {
					s += string(char) // TODO Try to use a buffer like bytes.NewBufferString(ret) for better performances
				}
			}
			mean = Welford_one_pass(mean, point, n_samples)
		}
		res[index] = mean
	}
	return res
}


func get_job_full_goroutine(request *Request) {

	// TODO probably it is needed to use the already connection which is in place for the heartbeat

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", request.Receiver.Ip + ":" + request.Receiver.Port)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	var reply int


	InfoLoggerPtr.Println("Requesting keys", request.Body, "from server", request.Receiver.Id)
	err = client.Call("Mapper_handler.Get_job_full", &request, &reply)
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}
}

func job_manager_goroutine(job_ptr *Job, chan_ptr *chan *Job) {

	requests_map := orderedmap.NewOrderedMap()
	keys_x_values := make(map[string]map[string]struct{})

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

	for el := requests_map.Front(); el != nil; el = el.Next() {
		el.Value.(*Request).Time = time.Now()
		go get_job_full_goroutine(el.Value.(*Request))
	}


	select {
		case job_full := <-Job_full_channel: // type: Request
		requests_map.Delete(job_full.Sender.Id)
		for i, v := range job_full.Body.(map[string]interface{}) {
			_, ok := keys_x_values[i]
			if !ok {
				keys_x_values[i] = v.(map[string]struct{})
			} else {
				for index, value2 := range v.(map[string]struct{}) {
					keys_x_values[i][index] = value2
				}
			}

		}

	//case time.After():
		// TODO Check for a time bound and retry,  
	}

	keys := make([]string, 1)

	// TODO check the error
	res, err := Call("reducer_algorithm_" + job_ptr.Algorithm, stub_storage, int(job_ptr.Properties_amount), keys,
		keys_x_values, job_ptr.Separate_properties, job_ptr.Algorithm_parameters)
	if err != nil { ErrorLoggerPtr.Fatal("Error calling reducer_algorithm:", err) }
	job_ptr.Result = res.(map[string]interface {})
	job_ptr.Keys = keys
	select {
	case *chan_ptr <- job_ptr:
	default:
		ErrorLoggerPtr.Fatal("Finished job channel full.")
	}
}

func send_completed_job_goroutine(job_ptr *Job, log_message string) {

	// TODO probably it is needed to use the already connection which is in place for the heartbeat

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", MASTER_IP + ":" + MASTER_PORT)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	var reply int

	err = client.Call("Master_handler.Job_reducer_completed", job_ptr, &reply)
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}
	InfoLoggerPtr.Println(log_message)
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
				select {
				case ready_event_channel <- struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("ready_event_channel is full.")
				}
			}

		case job_finished_ptr := <-*job_finished_channel_ptr:
			InfoLoggerPtr.Println("Job", job_finished_ptr.Id, "of task", job_finished_ptr.Task_id, "is finished.")
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

			go send_completed_job_goroutine(job_finished_ptr, "Sent completed job " + job_finished_ptr.Id + " of task " + job_finished_ptr.Task_id) // TODO add and manage errors

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

	gob.Register([]interface{}(nil))
	gob.Register(map[string]interface{}(nil))
	gob.Register(map[string]struct{}(nil))

	stub_storage = map[string]interface{}{
		"reducer_algorithm_clustering": reducer_algorithm_clustering,
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

	server = &Server{id, ip, REDUCER_PORT, time.Now(), nil, "REDUCER"}
}

func Reducer_main() {

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", MASTER_IP + ":" + MASTER_PORT)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
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

