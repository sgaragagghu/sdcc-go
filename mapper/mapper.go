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
	"math"
	"errors"
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

func get_actual_begin(load_ptr *[]byte, separate_entries byte) (int64, error) {
	reader := bytes.NewReader(*load_ptr)
	buffered_read := bufio.NewReader(reader)
	found := false
	var i int64 = 0
	// TODO, see the next function
	for char, err := buffered_read.ReadByte(); err == nil; char, err = buffered_read.ReadByte() {
		if char == separate_entries {
			found = true
			break
		}
		i += 1
	}
	if found == true {
		return i, nil
	} else { return i, errors.New("Separate entries not found") }
}

func get_actual_end(load_ptr *[]byte, separate_entries byte, offset int64) (int64, error) {
	reader := bytes.NewReader((*load_ptr)[offset - 1:]) //TODO check error
	buffered_read := bufio.NewReader(reader)
	found := false
	var i int64 = -1
	// TODO check if the buffer the fox stopped cause the buffer is empty but not the byte array
	for char, err := buffered_read.ReadByte(); err == nil; char, err = buffered_read.ReadByte() { // TODO check the error if != EOF
		if char == separate_entries {
			found = true
			break
		}
		i += 1
	}
	if found == true {
		return offset + i, nil
	} else { return offset + i, errors.New("Separate entries not found") }
}

func mapper_algorithm_clustering(properties_amount int, separate_entries byte, separate_properties byte, parameters []interface{}, load []byte) () {

	k := parameters[0].(int)

	u_vec := make([][]int, k)
	for i := range u_vec {
		//u_vec[i] = make([]int, properties_amount)
		u_vec[i] = parameters[i + 1].([]int)
	}

	reader := bytes.NewReader(load)
	buffered_read := bufio.NewReader(reader)

	for {
		j := 1
		s := ""
		point := make([]int, propetries_amount)
		var err error = nil
		var char byte = 0
		for char, err = buffered_read.ReadByte(); err != nil; char, err = buffered_read.ReadByte() {
			s += string(char) // TODO Try to use a buffer like bytes.NewBufferString(ret) for better performances
			if char == separate_properties {
				if j < (properties_amount - 1)  {
					point[j - 1], _ = strconv.Atoi(s) //TODO check the error
					s = ""
					j += 1
				} else { ErrorLoggerPtr.Fatal("Parsing failed") }
			} else if char == separate_entries {
				if j == (properties_amount - 1) {
					point[j - 1], _ = strconv.Atoi(s) // TODO check the error
					break
				} else { ErrorLoggerPtr.Fatal("Parsing failed") }
			}
		}
		min_index := 0
		min := -1
		for i := k - 1 ; i >= 0; i -= 1 {
			if distance := euclidean_norm(parameters_amount, u_vec[i], point); distance < min || min == -1 {
				min_index = i
				min = distance
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			} else {
				ErrorLoggerPtr.Fatal(err)
			}
		}
	}
}

func job_manager_goroutine(job_ptr *Job, chan_ptr *chan *Job) {

	// TODO check possible overflow
	load_ptr := Http_download(job_ptr.Resource_link, job_ptr.Begin, job_ptr.Begin + int64(math.Abs(float64((job_ptr.End - job_ptr.Begin)) * ((100 + float64(job_ptr.Margin))/100))))

	actual_begin, err := get_actual_begin(load_ptr, job_ptr.Separate_entries)
	if err != nil { ErrorLoggerPtr.Fatal("get_actual_begin error:", err) }

	actual_end, err := get_actual_end(load_ptr, job_ptr.Separate_entries, job_ptr.End - job_ptr.Begin)
	if err != nil { ErrorLoggerPtr.Fatal("get_actual_end error:", err) }
	// TODO check the error
	res, _ := Call("mapper_algorithm_" + job_ptr.Map_algorithm, stub_storage, job_ptr.Properties_amount,
		job_ptr.Map_algorithm_parameters, (*load_ptr)[actual_begin:actual_end])
	job_ptr.Result = res.(*map[int]interface {})

	select {
	case *chan_ptr <- job_ptr:
	default:
		ErrorLoggerPtr.Fatal("Finished job channel full.")
	}
}

func task_manager_goroutine() {

	state := IDLE
	task_hashmap := make(map[string]*list.List)
	ready_event_channel := make(chan struct{}, 1000)
	job_finished_channel := make(chan *Job, 1000)
	job_finished_channel_ptr := &job_finished_channel

	for {
		select {
		case job_ptr := <-*Job_channel_ptr:
			InfoLoggerPtr.Println("Received Task", job_ptr.Task_id, "job", job_ptr.Id)
			job_list_ptr, ok := task_hashmap[job_ptr.Task_id]
			if ok {
				job_list_ptr.PushBack(job_ptr)
			} else  {
				job_list_ptr = new(list.List)
				job_list_ptr.PushBack(job_ptr)
				task_hashmap[job_ptr.Task_id] = job_list_ptr
			}

			if state == IDLE {
				select {
				case ready_event_channel <- struct{}{}:
				default:
				ErrorLoggerPtr.Fatal("ready_event_channel is full.")
				}
			}

		case job_finished_ptr := <-*job_finished_channel_ptr:
			if job_list_ptr, ok := task_hashmap[job_finished_ptr.Task_id]; ok {
				job_list_ptr.Remove(job_list_ptr.Front())
				if job_list_ptr.Len() == 0 {
					delete (task_hashmap, job_finished_ptr.Task_id)
				}
			} else {
				ErrorLoggerPtr.Fatal("Finished job not found!")
			}

			if len(task_hashmap) > 0 {
				select{
				case ready_event_channel <- struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("ready_event_channel is full.")
				}
			}

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
						if min == -1 || min > task_id_int {
							min = task_id_int
						}
					}
					job_ptr := task_hashmap[strconv.Itoa(task_id_int)].Front().Value.(*Job)
					go job_manager_goroutine(job_ptr, job_finished_channel_ptr)
					state = BUSY
				} else {
					ErrorLoggerPtr.Fatal("Unexpected empty task hashmap.")
				}
			}
		}
	}
}

func init() {

	gob.Register([]interface{}(nil))

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
	job_channel := make(chan *Job, 1000)
	Job_channel_ptr = &job_channel

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

