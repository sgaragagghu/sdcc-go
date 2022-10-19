package master

import (
	. "../share"
	. "../rpc_master"
	. "../rpc_mapper"
	"net"
	"net/rpc"
	"reflect"
	"time"
	"encoding/gob"
	//"container/list"
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
	map_algorithm_parameters interface{}
//	shuffle_algorithm string
//	order_algorithm string
	reducer_amount int32
	reduce_algorithm string
}

var (
	// TODO both to be moved to the rpc file (if i'll make it)
	New_task_mapper_event_channel chan struct{}
	Task_mapper_channel chan *task

	add_mapper_channel chan *Server
	rem_mapper_channel chan *Server
)

func task_injector_goroutine() { // TODO make a jsonrpc interface to send tasks from a browser or curl 

	time.Sleep(60 * SECOND)

	parameters := make([]interface{}, 5)
	parameters[0] = 4 // k
	parameters[1] = []float64{0, 0} // u_0
	parameters[2] = []float64{1, 1} // u_1
	parameters[3] = []float64{2, 2} // u_2
	parameters[4] = []float64{3, 3} // u_3


/*
	parameters_ptr := new(list.List)
	parameters_ptr.PushFront(4) // k
	parameters_ptr.PushFront([]int{0, 0}) // u_0
	parameters_ptr.PushFront([]int{1, 1}) // u_1
	parameters_ptr.PushFront([]int{2, 2}) // u_2
	parameters_ptr.PushFront([]int{3, 3}) // u_3
*/
	task := task{-1, "https://raw.githubusercontent.com/sgaragagghu/sdcc-clustering-datasets/master/sdcc/2d-4c.csv", 1, 10, '\n', ',', 2, "clustering", parameters, 1, "clustering"}
	select {
	case Task_mapper_channel <- &task:
		select {
		case New_task_mapper_event_channel <- struct{}{}:
		default:
			ErrorLoggerPtr.Fatal("Task channel is full")
		}
	default:
		ErrorLoggerPtr.Fatal("Task channel is full")
	}

	InfoLoggerPtr.Println("Task correctly injected")
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
		case heartbeat_ptr := <-Heartbeat_channel:
			element_temp, ok := linked_hashmap.Get(heartbeat_ptr.Id)
			var server_temp_ptr *Server
			if ok {
				server_temp_ptr = element_temp.(*Server)
				server_temp_ptr.Last_heartbeat = heartbeat_ptr.Last_heartbeat
				if !linked_hashmap.Delete(heartbeat_ptr.Id) { // TODO Check: could work without deletion
					ErrorLoggerPtr.Fatal("Unexpected error")
				}
			} else {
				server_temp_ptr = heartbeat_ptr
				select {
				case add_mapper_channel <-server_temp_ptr:
				default:
					ErrorLoggerPtr.Fatal("Add_mapper_channel is full")
				}
			}
			linked_hashmap.Set(server_temp_ptr.Id, server_temp_ptr) // TODO is it efficient ? 
			//InfoLoggerPtr.Println("Received heartbeat from mapper:", server_temp_ptr.Id)
		case <-time.After(SECOND):
			for el := linked_hashmap.Front(); el != nil;  {
				server_temp_ptr := el.Value.(*Server)
				el = el.Next()
				if server_temp_ptr.Last_heartbeat.Unix() < time.Now().Unix() - EXPIRE_TIME {
					if !linked_hashmap.Delete(server_temp_ptr.Id) {
						ErrorLoggerPtr.Fatal("Unexpected error")
					}
					select {
					case rem_mapper_channel <-server_temp_ptr:
					default:
						ErrorLoggerPtr.Fatal("Add_mapper_channel is full")
					}
				}
			}
		}
	}
}

func scheduler_mapper_goroutine() {
	InfoLoggerPtr.Println("Scheduler_mapper_goroutine started.")

	var task_counter int32 = 0
	state := IDLE
	job_channel := make(chan *Job, 1000)
	idle_mapper_hashmap := make(map[string]*Server)
	working_mapper_hashmap := make(map[string]*Server)


	for {
		select {
		case rem_mapper_ptr := <-rem_mapper_channel:
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
							InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to mapper", job_ptr.Server_id)
							go send_job_goroutine(server_ptr, job_ptr)
							break
						}
					} else {
						select {
						case job_channel <- job_ptr:
							InfoLoggerPtr.Println("Job", job_ptr.Id, "rescheduled.")
						default:
							ErrorLoggerPtr.Fatal("job_channel queue full") // TODO handle this case...
						}
					}
				}
			delete(working_mapper_hashmap, rem_mapper_ptr.Id)
			}
		case add_mapper_ptr := <-add_mapper_channel:
			InfoLoggerPtr.Println("Mapper", add_mapper_ptr.Id, "is being added")
			mapper_job_map := make(map[string]*Job)
			add_mapper_ptr.Jobs = &mapper_job_map
			select {
			case job_ptr := <-job_channel:
				job_ptr.Server_id = add_mapper_ptr.Id
				working_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
				(*add_mapper_ptr.Jobs)[job_ptr.Id] = job_ptr
				InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to mapper", job_ptr.Server_id)
				go send_job_goroutine(add_mapper_ptr, job_ptr)
			default:
				idle_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
			}
		case job_completed_ptr := <-Job_mapper_completed_channel:
			mapper_job_map_ptr := working_mapper_hashmap[job_completed_ptr.Server_id].Jobs
			delete(*mapper_job_map_ptr, job_completed_ptr.Id)
			if len(*mapper_job_map_ptr) == 0 {
				select {
				case job_ptr := <-job_channel:
					job_ptr.Server_id = job_completed_ptr.Server_id
					InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to server", job_ptr.Server_id)
					(*mapper_job_map_ptr)[job_ptr.Id] = job_ptr
					go send_job_goroutine(working_mapper_hashmap[job_completed_ptr.Server_id], job_ptr)
				default:
					idle_mapper_hashmap[job_completed_ptr.Server_id] = working_mapper_hashmap[job_completed_ptr.Server_id]
					delete(working_mapper_hashmap, job_completed_ptr.Server_id)
				}
				if len(working_mapper_hashmap) == 0 {
					InfoLoggerPtr.Println("Task", job_completed_ptr.Id, "completed")
					state = IDLE
				}
				if state == IDLE && len(Task_mapper_channel) > 0 { // if the curent task finished and theres a task
					select {
					case New_task_mapper_event_channel <-struct{}{}:
					default:
						ErrorLoggerPtr.Fatal("New_task_mapper_event_channel full.")
					}
				}
			}
		case <-New_task_mapper_event_channel:
			if len(working_mapper_hashmap) == 0 { // if the curent task finished
				select {
				case task_ptr := <-Task_mapper_channel:
					task_ptr.id = task_counter
					task_counter += 1
					state = BUSY
					InfoLoggerPtr.Println("Scheduling task:", task_ptr.id)
					resource_size := Get_file_size(task_ptr.resource_link)
					mappers_amount := MinOf_int32(task_ptr.mappers_amount, int32(len(idle_mapper_hashmap))) // TODO check overflow
					if mappers_amount == 0 { mappers_amount = 1 }
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
					if len(idle_mapper_hashmap) > 0 {
						i := 0
						for _, server_ptr := range idle_mapper_hashmap {
							jobs[i].Server_id = server_ptr.Id
							working_mapper_hashmap[server_ptr.Id] = server_ptr
							server_ptr.Jobs[job_ptr.Id] = job_ptr
							go send_job_goroutine(server_ptr, jobs[i])
						i += 1
						}
					} else {
						select {
						case job_channel <- jobs[0]:
						default:
							ErrorLoggerPtr.Fatal("Job channel is full.")
						}
					}
				default:
				}
			}
		}
	}
}

func scheduler_reducer_goroutine() {
	InfoLoggerPtr.Println("Scheduler_reducer_goroutine started.")

	var task_counter int32 = 0
	state := IDLE
	job_channel := make(chan *Job, 1000)
	idle_reducer_hashmap := make(map[string]*Server)
	working_reducer_hashmap := make(map[string]*Server)


	for {
		select {
		case rem_reducer_ptr := <-rem_reducer_channel:
			if _, ok := idle_reducer_hashmap[rem_mapper_ptr.Id]; ok {
				delete(idle_reducer_hashmap, rem_reducer_ptr.Id)
			} else if server, ok := working_reducer_hashmap[rem_reducer_ptr.Id]; ok {
				jobs_ptr := server.Jobs
				for _, job_ptr := range *jobs_ptr {
					if len(idle_reducer_hashmap) > 0 {
						for server_id, server_ptr := range idle_reducer_hashmap {
							delete(idle_reducer_hashmap, server_id)
							job_ptr.Server_id = server_id
							working_reduer_hashmap[server_id] = server_ptr
							InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to reducer", job_ptr.Server_id)
							go send_job_goroutine(server_ptr, job_ptr)
							break
						}
					} else {
						select {
						case job_channel <- job_ptr:
							InfoLoggerPtr.Println("Job", job_ptr.Id, "rescheduled.")
						default:
							ErrorLoggerPtr.Fatal("job_channel queue full") // TODO handle this case...
						}
					}
				}
			delete(working_reducer_hashmap, rem_reduer_ptr.Id)
			}
		case add_reducer_ptr := <-add_reducer_channel:
			InfoLoggerPtr.Println("Reducer", add_mapper_ptr.Id, "is being added")
			reducer_job_map := make(map[string]*Job)
			add_reducer_ptr.Jobs = &reducer_job_map
			select {
			case job_ptr := <-job_channel:
				job_ptr.Server_id = add_reducer_ptr.Id
				working_reducer_hashmap[add_reducer_ptr.Id] = add_reducer_ptr
				(*add_reducer_ptr.Jobs)[job_ptr.Id] = job_ptr
				InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to reducer", job_ptr.Server_id)
				go send_job_goroutine(add_reducer_ptr, job_ptr)
			default:
				idle_reducer_hashmap[add_reducer_ptr.Id] = add_reducer_ptr
			}
		case job_completed_ptr := <-Job_mapper_completed_channel:
			mapper_job_map_ptr := working_mapper_hashmap[job_completed_ptr.Server_id].Jobs
			delete(*mapper_job_map_ptr, job_completed_ptr.Id)
			if len(*mapper_job_map_ptr) == 0 {
				select {
				case job_ptr := <-job_channel:
					job_ptr.Server_id = job_completed_ptr.Server_id
					InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to server", job_ptr.Server_id)
					(*mapper_job_map_ptr)[job_ptr.Id] = job_ptr
					go send_job_goroutine(working_mapper_hashmap[job_completed_ptr.Server_id], job_ptr)
				default:
					idle_mapper_hashmap[job_completed_ptr.Server_id] = working_mapper_hashmap[job_completed_ptr.Server_id]
					delete(working_mapper_hashmap, job_completed_ptr.Server_id)
				}
				if len(working_mapper_hashmap) == 0 {
					InfoLoggerPtr.Println("Task", job_completed_ptr.Id, "completed")
					state = IDLE
				}
				if state == IDLE && len(Task_mapper_channel) > 0 { // if the curent task finished and theres a task
					select {
					case New_task_mapper_event_channel <-struct{}{}:
					default:
						ErrorLoggerPtr.Fatal("New_task_mapper_event_channel full.")
					}
				}
			}
		case <-New_task_mapper_event_channel:
			if len(working_mapper_hashmap) == 0 { // if the curent task finished
				select {
				case task_ptr := <-Task_mapper_channel:
					task_ptr.id = task_counter
					task_counter += 1
					state = BUSY
					InfoLoggerPtr.Println("Scheduling task:", task_ptr.id)
					resource_size := Get_file_size(task_ptr.resource_link)
					mappers_amount := MinOf_int32(task_ptr.mappers_amount, int32(len(idle_mapper_hashmap))) // TODO check overflow
					if mappers_amount == 0 { mappers_amount = 1 }
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
					if len(idle_mapper_hashmap) > 0 {
						i := 0
						for _, server_ptr := range idle_mapper_hashmap {
							jobs[i].Server_id = server_ptr.Id
							working_mapper_hashmap[server_ptr.Id] = server_ptr
							server_ptr.Jobs[job_ptr.Id] = job_ptr
							go send_job_goroutine(server_ptr, jobs[i])
						i += 1
						}
					} else {
						select {
						case job_channel <- jobs[0]:
						default:
							ErrorLoggerPtr.Fatal("Job channel is full.")
						}
					}
				default:
				}
			}
		}
	}
}


func Master_main() {

	gob.Register([]interface{}(nil))

	// creating channel for communicating the heartbeat
	// to the goroutine heartbeat manager
	Heartbeat_channel = make(chan *Server, 1000)


	// creating channel for communicating the connected
	// and disconnected workers to the scheduler
	add_mapper_channel = make(chan *Server, 1000)
	rem_mapper_channel = make(chan *Server, 1000)

	//creating channel for communicating ended jobs
	Job_mapper_completed_channel = make(chan *Job, 1000)

	//creating channel for communicating new task event
	New_task_mapper_event_channel = make(chan struct{}, 1000)

	//creating channel for communicating new task
	Task_mapper_channel = make(chan *task, 1000)

	go scheduler_mapper_goroutine()
	go heartbeat_goroutine()
	go task_injector_goroutine()


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

