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

var (
	add_mapper_channel_ptr *chan *Server
	rem_mapper_channel_ptr *chan *Server
)


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
						//- call a function to send the job to the server
						//- assign the job to the first
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
			add_mapper_ptr = add_mapper_ptr
		case job_completed_ptr := <-*Job_completed_channel_ptr:
			job_completed_ptr = job_completed_ptr
			select {
			case job_ptr := <-job_channel:
				//- send it to the server which has just completed the job
			default:
			}
			if len(working_mapper_hashmap) == 0 && len(*Task_channel_ptr) > 0 { // if the curent task finished and theres a task
				/*
				select {
				case New_task_event_channel_ptr <-[TODO: HERE CREATE THE EVENT]:
				default:
				}
				*/
			}
		case new_task_event_ptr := <-*New_task_event_channel_ptr:
			new_task_event_ptr=new_task_event_ptr
			if len(working_mapper_hashmap) == 0 { // if the curent task finished
				select {
				case task := <-*Task_channel_ptr:
					// start the new task...
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

