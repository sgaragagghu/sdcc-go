package master

import (
	. "../share"
	. "../rpc_master"
	. "../rpc_mapper"
	"net"
	"net/rpc"
	"reflect"
	"time"

	"github.com/elliotchance/orderedmap"
)

type task struct {
	id int32
	resource_link string
	mappers_amount int32
	margin int32
	separate_entries byte
	separate properties byte
	map_algorithm string
//	shuffle_algorithm string
//	order_algorithm string
	reducer_amount int32
	reduce_algorithm string
}

var (
	add_mapper_channel_ptr *chan *Server
	rem_mapper_channel_ptr *chan *Server
)

func task_injector() { // TODO make a jsonrpc interface to send tasks from a browser or curl 

	time.Sleep(60 * SECOND)
	task := task{"", "LINK...", 1, 10, '\n', ',' "clustering", 1, "clustering"}
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

func send_job() {
	// TODO
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

	task_counter int32 := 0
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
							go send_job(server_ptr, job_ptr)
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
				working_mapper_hashmap[add_mapper_ptr.Id] = server_ptr
				go send_job(add_mapper_ptr, job_ptr)
			default:
				idle_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
			}
		case job_completed_ptr := <-*Job_completed_channel_ptr:
			select {
			case job_ptr := <-job_channel:
				job_ptr.Server_id = job_completed_ptr.Server_id
				go send_job(working_mapper_hashmap[job_completed_ptr.Server_id], job_ptr)
			default:
			}
			if len(working_mapper_hashmap) == 0 && len(*Task_channel_ptr) > 0 { // if the curent task finished and theres a task
				select {
				case New_task_event_channel_ptr <-struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("New_task_event_channel full.")
				}
			}
		case _ := <-*New_task_event_channel_ptr:
			if len(working_mapper_hashmap) == 0 { // if the curent task finished
				select {
				case task_ptr := <-*Task_channel_ptr:
					task_ptr.id = Task_counter++
					resource_size = Get_file_size(task_ptr.resource_link)
					mappers_amount := MinOf(task_ptr.mappers_amount, len(idle_mapper_hashmap)
					slice_size := Abs(resource_size / mappers_amount) // maybe math.Abs
					jobs *Job[mappers_amount]
					for i := 0; i < mappers_amount; ++i {
						begin := i * slice_size
						end := (i + 1) * slize_size
						if i == 0 begin = 0
						if i == mappers_amount end = resource_size

						jobs[i] = &Job{i, task_ptr.id, "", task_ptr.resource_link, begin, end, task_ptr.margin,	\
							task_ptr.separate_entries, task_ptr.separate_properties, map_algorithm, begin, end}
					}
					{
						var i := 0
						for _, server_ptr := range idle_mapper_hashmap {
							working_mapper_hashmap[server_ptr.Id] = server_ptr
							go send_job(server_ptr, jobs[i])
						++i
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

	add_mapper_channel_ptr = add_mapper_channel_ptr
	rem_mapper_channel_ptr = rem_mapper_channel_ptr

	//creating channel for communicating ended jobs
	job_completed_channel := make(chan *Server, 1000)
	Job_completed_channel_ptr = &job_completed_channel

	Job_completed_channel_ptr = Job_completed_channel_ptr

	//creating channel for communicating new task event
	new_task_event_channel := make(chan *Server, 1000)
	New_task_event_channel_ptr = &new_task_event_channel

	New_task_event_channel_ptr = New_task_event_channel_ptr

	//creating channel for communicating new task
	task_channel := make(chan *Server, 1000)
	Task_channel_ptr = &task_channel

	Task_channel_ptr = Task_channel_ptr

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

