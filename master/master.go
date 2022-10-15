package master

import (
	. "../share"
	. "../rpc_master"
	. "../rpc_mapper"
	"net"
	"net/rpc"
	"reflect"
	"time"
	"container/list"
	"math"
	"strconv"

	"github.com/elliotchance/orderedmap"
)

type task struct {
	id int32
	resource_link string
	mappers_amount int32
	margin int8
	separate_entries byte
	separate_properties byte
	properties_amount int8
	map_algorithm string
	map_algorithm_parameters *list.List
//	shuffle_algorithm string
//	order_algorithm string
	reducer_amount int32
	reduce_algorithm string
}

var (
	// TODO both to be moved to the rpc file (if i'll make it)
	New_task_event_channel_ptr *chan struct{}
	Task_channel_ptr *chan *task

	add_mapper_channel_ptr *chan *Server
	rem_mapper_channel_ptr *chan *Server
)

func task_injector() { // TODO make a jsonrpc interface to send tasks from a browser or curl 

	time.Sleep(60 * SECOND)
	parameters_ptr := new(list.List)
	parameters_ptr.PushFront(4) // k
	parameters_ptr.PushFront([]int{0, 0}) // u_0
	parameters_ptr.PushFront([]int{1, 1}) // u_1
	parameters_ptr.PushFront([]int{2, 2}) // u_2
	parameters_ptr.PushFront([]int{3, 3}) // u_3

	task := task{-1, "LINK...", 1, 10, '\n', ',', 2, "clustering", parameters_ptr, 1, "clustering"}
	select {
	case *Task_channel_ptr <- &task:
		select {
		case *New_task_event_channel_ptr <- struct{}{}:
		default:
			ErrorLoggerPtr.Fatal("Task channel is full")
		}
	default:
		ErrorLoggerPtr.Fatal("Task channel is full")
	}
}

func send_job_goroutine(server_ptr *Server, job_ptr *Job) {
	// connect to mapper via rpc tcp
	client, err := rpc.Dial("tcp", server_ptr.Ip + ":" + server_ptr.Port)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	var reply int

	err = client.Call("Mapper_handler.Send_job", job_ptr, &reply)
	if err != nil {
		ErrorLoggerPtr.Fatal("Send_job error:", err)
	}
	InfoLoggerPtr.Println("Job sent.")

}

func heartbeat_goroutine() {

	InfoLoggerPtr.Println("Heartbeat_goroutine started.")

	linked_hashmap := orderedmap.NewOrderedMap()

	for {
		select {
		case heartbeat_ptr := <-*Heartbeat_channel_ptr:
			element_temp := linked_hashmap.GetElement(heartbeat_ptr.Id)
			var server_temp_ptr *Server
			if element_temp != nil {
				server_temp_ptr = element_temp.Value.(*Server)
				server_temp_ptr.Last_heartbeat = heartbeat_ptr.Last_heartbeat
				if !linked_hashmap.Delete(heartbeat_ptr.Id) { // TODO Check: could work without deletion
					ErrorLoggerPtr.Fatal("Unexpected error")
				}
			} else {
				// TODO write on chan that there's a new server
				server_temp_ptr = heartbeat_ptr
			}
			linked_hashmap.Set(server_temp_ptr.Id, server_temp_ptr) // TODO is it efficient ? 
			//InfoLoggerPtr.Println("received heartbeat")
		case <-time.After(SECOND):
			for el := linked_hashmap.Front(); el != nil;  {
				server_temp_ptr := el.Value.(*Server)
				el = el.Next()
				if server_temp_ptr.Last_heartbeat.Unix() > time.Now().Unix() - EXPIRE_TIME {
					if !linked_hashmap.Delete(server_temp_ptr.Id) {
						ErrorLoggerPtr.Fatal("Unexpected error")
					}
					// TODO write on chan that this server is dead
				}
			}
		}
	}
}

func scheduler_mapper_goroutine() {
	InfoLoggerPtr.Println("Scheduler_mapper_goroutine started.")

	var task_counter int32 = 0
	job_channel := make(chan *Job, 1000)
	idle_mapper_hashmap := make(map[string]*Server)
	working_mapper_hashmap := make(map[string]*Server)


	for {
		select {
		case rem_mapper_ptr := <-*rem_mapper_channel_ptr:
			if _, ok := idle_mapper_hashmap[rem_mapper_ptr.Id]; ok {
				delete(idle_mapper_hashmap, rem_mapper_ptr.Id)
			} else if server, ok := working_mapper_hashmap[rem_mapper_ptr.Id]; ok {
				jobs_ptr := server.Jobs
				for _, job_ptr := range *jobs_ptr {
					if len(idle_mapper_hashmap) > 0 {
						for server_id, server_ptr := range idle_mapper_hashmap {
							delete(idle_mapper_hashmap, server_id)
							job_ptr.Server_id = server_id
							working_mapper_hashmap[server_id] = server_ptr
							go send_job_goroutine(server_ptr, job_ptr)
							break
						}
					} else {
						select {
						case job_channel <- job_ptr:
						default:
							ErrorLoggerPtr.Fatal("job_channel queue full") // TODO handle this case...
						}
					}
				}
			delete(working_mapper_hashmap, rem_mapper_ptr.Id)
			}
		case add_mapper_ptr := <-*add_mapper_channel_ptr:
			select {
			case job_ptr := <-job_channel:
				job_ptr.Server_id = add_mapper_ptr.Id
				working_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
				go send_job_goroutine(add_mapper_ptr, job_ptr)
			default:
				idle_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
			}
		case job_completed_ptr := <-*Job_completed_channel_ptr:
			select {
			case job_ptr := <-job_channel:
				job_ptr.Server_id = job_completed_ptr.Server_id
				go send_job_goroutine(working_mapper_hashmap[job_completed_ptr.Server_id], job_ptr)
			default:
			}
			if len(working_mapper_hashmap) == 0 && len(*Task_channel_ptr) > 0 { // if the curent task finished and theres a task
				select {
				case *New_task_event_channel_ptr <-struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("New_task_event_channel full.")
				}
			}
		case <-*New_task_event_channel_ptr:
			if len(working_mapper_hashmap) == 0 { // if the curent task finished
				select {
				case task_ptr := <-*Task_channel_ptr:
					task_ptr.id = task_counter
					task_counter += 1
					resource_size := Get_file_size(task_ptr.resource_link)
					mappers_amount := MinOf_int32(task_ptr.mappers_amount, int32(len(idle_mapper_hashmap))) // TODO check overflow
					slice_size := int64(math.Abs(float64(resource_size) / float64(mappers_amount)))
					jobs := make([]*Job, mappers_amount)
					{
						var i int32 = 0
						for ; i < mappers_amount; i += 1 {
							begin := int64(i) * slice_size
							end := (int64(i) + 1) * slice_size
							if i == 0 { begin = 0 }
							if i == mappers_amount { end = resource_size }

							jobs[i] = &Job{strconv.FormatInt(int64(i), 10), strconv.FormatInt(int64(task_ptr.id), 10),
								"", task_ptr.resource_link, begin, end, task_ptr.margin,
								task_ptr.separate_entries, task_ptr.separate_properties, task_ptr.properties_amount,
								task_ptr.map_algorithm, task_ptr.map_algorithm_parameters, nil}
						}
					}
					{
						i := 0
						for _, server_ptr := range idle_mapper_hashmap {
							working_mapper_hashmap[server_ptr.Id] = server_ptr
							go send_job_goroutine(server_ptr, jobs[i])
						i += 1
						}
					}
				default:
				}
			}
		}
	}
}

func master_main() {

	// creating channel for communicating the heartbeat
	// to the goroutine heartbeat manager
	heartbeat_channel := make(chan *Server, 1000)
	Heartbeat_channel_ptr = &heartbeat_channel


	// creating channel for communicating the connected
	// and disconnected workers to the scheduler
	add_mapper_channel := make(chan *Server, 1000)
	add_mapper_channel_ptr = &add_mapper_channel
	rem_mapper_channel := make(chan *Server, 1000)
	rem_mapper_channel_ptr = &rem_mapper_channel

	//creating channel for communicating ended jobs
	job_completed_channel := make(chan *Job, 1000)
	Job_completed_channel_ptr = &job_completed_channel

	//creating channel for communicating new task event
	new_task_event_channel := make(chan struct{}, 1000)
	New_task_event_channel_ptr = &new_task_event_channel

	//creating channel for communicating new task
	task_channel := make(chan *task, 1000)
	Task_channel_ptr = &task_channel

	go scheduler_mapper_goroutine()
	go heartbeat_goroutine()

	master_handler := new(Master_handler)

	// register Master_handler as RPC interface
	rpc.Register(master_handler)

	// service address of server
	service := ":" + MASTER_PORT

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
		// handle tcp client connections
		conn, err := listener.Accept()
		if err != nil {
			WarningLoggerPtr.Println("listener accept error:", err)
		}

		// print connection info
		InfoLoggerPtr.Println("received message", reflect.TypeOf(conn), conn)

		// handle client connections via rpc
		go rpc.ServeConn(conn)
	}

}

