package master

import (
	. "../share"
	. "../rpc_master"
	//. "../rpc_mapper"
	"net"
	go_rpc "net/rpc"
	"net/http"
	//"net/rpc/jsonrpc"
	"reflect"
	"time"
	"encoding/gob"
	//"container/list"
	"math"
	"math/rand"
	"strconv"
	"bytes"
	"bufio"
	"fmt"
	"errors"

	"github.com/elliotchance/orderedmap"

	"github.com/gorilla/mux"
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
)

// Usedby the map scheduler to inform the reducer scheduler that keys_x_servers has been updated
type map_to_reduce_error struct {
	task_id string
	keys_x_servers *orderedmap.OrderedMap
}

var (
	New_task_mapper_event_channel chan struct{}
	New_task_reducer_event_channel chan struct{}
	Task_mapper_channel chan *Task
	Task_reducer_channel chan *Task

	add_mapper_channel chan *Server
	rem_mapper_channel chan *Server
	add_reducer_channel chan *Server
	rem_reducer_channel chan *Server
	// channel to send new keys_x_servers to the reducer scheduler
	probable_reducer_error_channel chan *map_to_reduce_error
	task_reducer_completed_channel chan string

	// data structure for calling functions by its name in a string
	stub_storage StubMapping
)
// "custom" initialization algorithm
func initialization_algorithm_clustering(task_ptr *Task) (ret error) {
	ret = nil
	resource_size := Get_file_size(task_ptr.Resource_link)

	if Check_float64_to_int_overflow(task_ptr.Iteration_algorithm_parameters.([]interface{})[0].(float64)) { ErrorLoggerPtr.Println("Overflow!!") }
	task_ptr.Iteration_algorithm_parameters.([]interface{})[0] = int(task_ptr.Iteration_algorithm_parameters.([]interface{})[0].(float64))
	if Check_float64_to_int_overflow(task_ptr.Map_algorithm_parameters.([]interface{})[0].(float64)) { ErrorLoggerPtr.Println("Overflow!!") }
	task_ptr.Map_algorithm_parameters.([]interface{})[0] = int((task_ptr.Map_algorithm_parameters.([]interface{})[0]).(float64))

	// checking if the task is valid
	var missing = ""
	if task_ptr.Resource_link == "" { missing = "resource_link" }
	if task_ptr.Mappers_amount == 0 { missing = "mappers_amount" }
	if task_ptr.Margin == 0 { missing = "marign" }
	if len(string(task_ptr.Separate_entries)) != 1 { missing = "separate_entries" }
	if len(string(task_ptr.Separate_properties)) != 1 { missing = "separate_properties" }
	if task_ptr.Properties_amount == 0 { missing = "properties_amount" }
	if task_ptr.Initialization_algorithm == "" { missing = "initialization_algorithm" }
	if task_ptr.Map_algorithm == "" { missing = "map_algorithm" }
	if task_ptr.Map_algorithm_parameters == nil || len(task_ptr.Map_algorithm_parameters.([]interface{})) == 0 { missing = "map_algorithm_parameters" }
	if task_ptr.Reducers_amount == 0 { missing = "reducers_amount" }
	if task_ptr.Reduce_algorithm == "" { missing = "reduce_algorithm" }
	if task_ptr.Iteration_algorithm == "" { missing = "iteration_algorithm" }
	if task_ptr.Iteration_algorithm_parameters == nil || len(task_ptr.Map_algorithm_parameters.([]interface{})) == 0 { missing = "iteration_algorithm_parameters" }

	if task_ptr.Id != "-1" { missing = "start Id" }
	if task_ptr.Origin_id != "-1" { missing = "start Origin_id" }
	if task_ptr.Send_time != 0 { missing = "start Send_time" }
	if task_ptr.Keys_x_servers == nil { missing = "Keys_x_servers" }
	if task_ptr.Keys_x_servers_version != 0 { missing = "start Keys_x_servers_version" }
	if task_ptr.Jobs == nil { missing = "start Jobs" }
	if task_ptr.Jobs_done == nil { missing = "start Jobs_done" }

	if missing != "" {
		ret = fmt.Errorf("Missing: %v", missing)
		return
	}


	// variable which will contain the random starting centroids
	offsets := make([][]float64, task_ptr.Map_algorithm_parameters.([]interface{})[0].(int))

	// for each random centroid, it will download a random slice of the file and then find the first point and pick it as centroid
	for i, _ := range offsets {
		offsets[i] = make ([]float64, 2)
		if Check_float64_to_int_overflow(math.Abs(((float64(resource_size) / float64(task_ptr.Mappers_amount)) / 100) * float64(task_ptr.Margin))) {
			ErrorLoggerPtr.Println("Overflow!!")
		}
		// size of the slice, it is as big as the margin size experimented by every mapper
		download_size := int(math.Abs(((float64(resource_size) / float64(task_ptr.Mappers_amount)) / 100) * float64(task_ptr.Margin)))
		if resource_size > MaxInt { ErrorLoggerPtr.Println("Overflow!") }
		offset := rand.Intn(int(resource_size) - download_size)

		load_ptr := Http_download(task_ptr.Resource_link, int64(offset), int64(offset + download_size))

		actual_begin, err := Get_actual_begin(load_ptr, task_ptr.Separate_entries)
		if err != nil {
			ret = fmt.Errorf("get_actual_begin error: %v", err)
			return
		}

		actual_end, err := Get_actual_end(load_ptr, task_ptr.Separate_entries, actual_begin + 1)
		if err != nil {
			ret = fmt.Errorf("get_actual_end error: %v", err)
			return
		}

		//InfoLoggerPtr.Println("Actual begin:", actual_begin, "actual end:", actual_end)

		if actual_begin == actual_end {
			ret = fmt.Errorf("Unexpected error")
			return
		}


		reader := bytes.NewReader((*load_ptr)[actual_begin:actual_end + 1])
		buffered_read := bufio.NewReader(reader)
		Parser_simple(&(offsets[i]), buffered_read, task_ptr.Separate_properties, task_ptr.Separate_entries)
	}

	// printing the random centroids
	InfoLoggerPtr.Println("Seed:")
	for i, v := range offsets {
		if i >= len(task_ptr.Map_algorithm_parameters.([]interface{})) - 1 {
			task_ptr.Map_algorithm_parameters = append(task_ptr.Map_algorithm_parameters.([]interface{}), v)
		} else {
			task_ptr.Map_algorithm_parameters.([]interface{})[i + 1] = v
		}

		InfoLoggerPtr.Println(v)
	}

	return
}

func task_injector_goroutine() {

	// Test task

	time.Sleep(60 * SECOND)

	parameters := make([]interface{}, 5)
	parameters[0] = float64(4) // k
	parameters[1] = []float64{0, 0} // u_0
	parameters[2] = []float64{1, 1} // u_1
	parameters[3] = []float64{2, 2} // u_2
	parameters[4] = []float64{3, 3} // u_3


	iteration_parameters := make([]interface{}, 1)
	iteration_parameters[0] =  float64(2) // max_diff (percentage)

	task_ptr := &Task{
		Id:"-1",
		Origin_id:"-1",
		Send_time:0,
		Resource_link:"https://raw.githubusercontent.com/sgaragagghu/sdcc-clustering-datasets/master/sdcc/2d-4c.csv",
		Mappers_amount:2,
		Margin:10,
		Separate_entries:'\n',
		Separate_properties:',',
		Properties_amount:2,
		Initialization_algorithm:"clustering",
		Map_algorithm:"clustering",
		Map_algorithm_parameters:parameters,
		Reducers_amount:2,
		Reduce_algorithm:"clustering",
		Reduce_algorithm_parameters:nil,
		Join_algorithm:"clustering",
		Join_algorithm_parameters:nil,
		Iteration_algorithm:"clustering",
		Iteration_algorithm_parameters:iteration_parameters,
		Keys_x_servers:orderedmap.NewOrderedMap(),
		Keys_x_servers_version:0,
		Jobs:make(map[string]*Job),
		Jobs_done:make(map[string]*Job),
	}

	err1, err2 := Call("initialization_algorithm_" + task_ptr.Initialization_algorithm, stub_storage, task_ptr)

	if err1 != nil || err2 != nil {
		ErrorLoggerPtr.Fatal("iniziatialization error:", err1, err2)
	}


/*
	for index, index_value_o := range task_ptr.Map_algorithm_parameters.([]interface{})[1:] {
		index_value := index_value_o.([]float64)
		InfoLoggerPtr.Println("key", index, "value", index_value)
	}
*/

	select {
	case Task_mapper_channel <- task_ptr:
		select {
		case New_task_mapper_event_channel <- struct{}{}:
		default:
			ErrorLoggerPtr.Fatal("Task channel is full")
		}
	default:
		ErrorLoggerPtr.Fatal("Task channel is full")
	}

	InfoLoggerPtr.Println("Task correctly injected")

	// here receiving tasks from the user
	for {
		select {
		case slice := <-Task_from_JRPC_channel:
			var reply error = nil
			task_ptr := ((*slice)[1]).(*Task)
			// checking and initializing the the task
			err1, err2 := Call("initialization_algorithm_" + task_ptr.Initialization_algorithm, stub_storage, task_ptr)

			if err1 != nil { reply = err1.(error) }
			if err2 != nil { reply = err2 }

			if reply == nil {
				select {
				case Task_mapper_channel <- task_ptr:
					select {
					case New_task_mapper_event_channel <- struct{}{}:
						InfoLoggerPtr.Println("Task correctly injected")
					default:
						WarningLoggerPtr.Fatal("Task channel is full")
						reply = errors.New("Task channel is full")
					}
				default:
					WarningLoggerPtr.Fatal("Task channel is full")
					reply = errors.New("Task channel is full")
				}
			}
			select {
			case (*slice)[0].(chan error)<-reply:
			default:
				WarningLoggerPtr.Println("Reply channel is full.")
			}
		}
	}
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
				// if the server is alredy present, then update the last heartbeat time
				server_temp_ptr = element_temp.(*Server)
				server_temp_ptr.Last_heartbeat = heartbeat_ptr.Last_heartbeat
				if !linked_hashmap.Delete(heartbeat_ptr.Id) { // TODO Check: could work without deletion
					ErrorLoggerPtr.Fatal("Unexpected error")
				}
			} else {
				// if the server is new, then add it (mapper or reducer)
				server_temp_ptr = heartbeat_ptr
				if server_temp_ptr.Role == MAPPER {
					select {
					case add_mapper_channel <-server_temp_ptr:
						Status_ptr.Mapper_amount += 1
					default:
						ErrorLoggerPtr.Fatal("Add_mapper_channel is full")
					}
				} else if server_temp_ptr.Role == REDUCER {
					select {
					case add_reducer_channel <-server_temp_ptr:
						Status_ptr.Reducer_amount += 1
					default:
						ErrorLoggerPtr.Fatal("Add_reducer_channel is full")
					}
				} else { ErrorLoggerPtr.Fatal("Unexpected kind of role:", server_temp_ptr.Role) }
			}
			linked_hashmap.Set(server_temp_ptr.Id, server_temp_ptr) // TODO is it efficient ?
			//InfoLoggerPtr.Println("Received heartbeat from mapper:", server_temp_ptr.Id)
		case <-time.After(SECOND):
			// every second is checking if some worker is not sending any heartbeat for more than
			// EXPIRE_TIME (they should send it every EXPIRE_TIME / 2)
			for el := linked_hashmap.Front(); el != nil;  {
				server_temp_ptr := el.Value.(*Server)
				el = el.Next()
				if server_temp_ptr.Last_heartbeat.Unix() < time.Now().Unix() - EXPIRE_TIME {
					if !linked_hashmap.Delete(server_temp_ptr.Id) {
						ErrorLoggerPtr.Fatal("Unexpected error")
					}
					if server_temp_ptr.Role == MAPPER {
						select {
						case rem_mapper_channel <-server_temp_ptr:
							Status_ptr.Mapper_amount -= 1
						default:
							ErrorLoggerPtr.Fatal("Rem_mapper_channel is full")
						}
					} else if server_temp_ptr.Role == REDUCER {

						select {
						case rem_reducer_channel <-server_temp_ptr:
							Status_ptr.Reducer_amount -= 1
						default:
							ErrorLoggerPtr.Fatal("Rem_reducer_channel is full")
						}
					}
				}
			}
		}
	}
}

// Just sending the job to the mapper and update the data structures
func assign_job_mapper(server_ptr *Server, job_ptr *Job, working_mapper_hashmap map[string]*Server, servers_x_tasks_x_jobs map[string]map[string]map[string]*Job) {

	job_ptr.Server_id = server_ptr.Id
	working_mapper_hashmap[server_ptr.Id] = server_ptr
	{
		job_map, ok := servers_x_tasks_x_jobs[server_ptr.Id][job_ptr.Task_id]
		if !ok {
			job_map = make(map[string]*Job)
			servers_x_tasks_x_jobs[server_ptr.Id][job_ptr.Task_id] = job_map
		}
		job_map[job_ptr.Id] = job_ptr
	}
	InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to mapper", job_ptr.Server_id)
	go Rpc_job_goroutine(server_ptr, job_ptr, "Mapper_handler.Send_job",
		"Sent mapper job " + job_ptr.Id + " [begin " + fmt.Sprint(job_ptr.Begin) + ", end " + fmt.Sprint(job_ptr.End) + "] task " + job_ptr.Task_id,
		3, EXPIRE_TIME, false)
}

// Maximum 2 task, one done (and excecuting the reduce phase) and one executing mapper phase
func scheduler_mapper_goroutine() {
	InfoLoggerPtr.Println("Scheduler_mapper_goroutine started.")

	var task_counter int32 = 0
	job_channel := make(chan *Job, 1000)
	idle_mapper_hashmap := make(map[string]*Server)
	working_mapper_hashmap := make(map[string]*Server)
	task_hashmap := orderedmap.NewOrderedMap()
	servers_x_tasks_x_jobs := make(map[string]map[string]map[string]*Job)
	servers_x_tasks_x_jobs_done := make(map[string]map[string]map[string]*Job)

	for {
		select {
		case rem_mapper_ptr := <-rem_mapper_channel:
			InfoLoggerPtr.Println("Mapper", rem_mapper_ptr.Id, "ip", rem_mapper_ptr.Ip, "port", rem_mapper_ptr.Port, "is being removed.")
			// removing a mapper
			delete(idle_mapper_hashmap, rem_mapper_ptr.Id)
			// data structure to store the new keys_x_servers variable
			probable_reducer_task_error := make(map[string]struct{})
			for el := task_hashmap.Back(); el != nil; el = el.Prev() { // checking each task for jobs that have to be rescheduled (there are maximum 2 tasks)
				current_task_ptr := el.Value.(*Task).Id
				task_ptr := el.Value.(*Task)
				jobs := servers_x_tasks_x_jobs[rem_mapper_ptr.Id][current_task_ptr]
				for i := 0; i < 2; i += 1 { // 2 rounds, one for the to-do jobs and one for the done jobs
					for _, job_ptr := range jobs {
						if i > 0 { // this is only done in the iteration with the already done jobs
							// rescheduling already done jobs which result was on the crashed mapper
							probable_reducer_task_error[job_ptr.Task_id] = struct{}{}
							delete(task_ptr.Jobs_done, job_ptr.Id)
							task_ptr.Jobs[job_ptr.Id] = job_ptr
							for _, v := range job_ptr.Keys { // checking each key of the job...
								servers, found := task_ptr.Keys_x_servers.Get(v)
								if !found { ErrorLoggerPtr.Fatal("Key not found.") }
								delete(servers.(map[string]*Server), rem_mapper_ptr.Id)
							}
							WarningLoggerPtr.Println("Mapper", rem_mapper_ptr.Id, "disconnected, previously done job", job_ptr.Id, "task", task_ptr.Id,
									"is being rescheduled.")
						} else {
							WarningLoggerPtr.Println("Mapper", rem_mapper_ptr.Id, "disconnected, job", job_ptr.Id, "task", task_ptr.Id,
									"is being rescheduled.")
						}
						// if there is some idle mapper then assign the job, otherwise add it to the to-do job channel (kind of a queue)
						if len(idle_mapper_hashmap) > 0 {
							for _, server_ptr := range idle_mapper_hashmap {
								delete(idle_mapper_hashmap, server_ptr.Id)
								assign_job_mapper(server_ptr, job_ptr, working_mapper_hashmap, servers_x_tasks_x_jobs)
								break
							}
						} else {
							select {
							case job_channel <- job_ptr:
								InfoLoggerPtr.Println("Mapper job", job_ptr.Id, "rescheduled.")
							default:
								ErrorLoggerPtr.Fatal("job_channel queue full")
							}
						}
					}
					// second iteration will consider the already done jobs
					jobs = servers_x_tasks_x_jobs_done[rem_mapper_ptr.Id][current_task_ptr]
				}
			}
			// notify the reducer scheduler an error could occur because a mapper crashed
			for task_id, _ := range probable_reducer_task_error {
				select {
				case probable_reducer_error_channel <- &map_to_reduce_error{task_id, nil}:
				default:
					ErrorLoggerPtr.Fatal("Probable_reducer_error_channel is full.")
				}
			}
			delete(working_mapper_hashmap, rem_mapper_ptr.Id)
			delete(servers_x_tasks_x_jobs, rem_mapper_ptr.Id)
			delete(servers_x_tasks_x_jobs_done, rem_mapper_ptr.Id)
		// adding a new mapper
		case add_mapper_ptr := <-add_mapper_channel:
			InfoLoggerPtr.Println("Mapper", add_mapper_ptr.Id, "ip", add_mapper_ptr.Ip, "port", add_mapper_ptr.Port, "is being added.")
			// preparing data structures
			servers_x_tasks_x_jobs[add_mapper_ptr.Id] = make(map[string]map[string]*Job)
			servers_x_tasks_x_jobs_done[add_mapper_ptr.Id] = make(map[string]map[string]*Job)
			// since there is a new idle mapper, try to assign a job (if any)
			for loop := true; loop; { // loop because there can be old job which task is actually already completed (this can happen when a mapper has crashed, the job has been rescheduled but the reduce already finished its job and the mapper didn't know it)
				select {
				case job_ptr := <-job_channel: // TODO prioritize oldest task's jobs first
					if _, present := task_hashmap.Get(job_ptr.Task_id); !present { continue }
					assign_job_mapper(add_mapper_ptr, job_ptr, working_mapper_hashmap, servers_x_tasks_x_jobs)
				default:
					idle_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
				}
				loop = false
			}
		// job has been completed, so we have to check if perhaps a task is finished
		case job_completed_ptr := <-Job_mapper_completed_channel:
			task_ptr_o, present := task_hashmap.Get(job_completed_ptr.Task_id)
			if !present { break } // Same case as { continue } few line above
			{	// receiveing a job from a removed server
				_, ok := working_mapper_hashmap[job_completed_ptr.Server_id]
				if !ok {
					_, ok = idle_mapper_hashmap[job_completed_ptr.Server_id]
					if !ok { break }
				}
			}
			task_ptr := task_ptr_o.(*Task)
			// moving the job from the to-do jobs data structure to the done jobs data structure
			{
				job_map, ok := servers_x_tasks_x_jobs_done[job_completed_ptr.Server_id][job_completed_ptr.Task_id]
				if !ok {
					job_map = make(map[string]*Job)
					servers_x_tasks_x_jobs_done[job_completed_ptr.Server_id][job_completed_ptr.Task_id] = job_map
				}
				job_map[job_completed_ptr.Id] = job_completed_ptr
			}

			delete(task_ptr.Jobs, job_completed_ptr.Id)
			task_ptr.Jobs_done[job_completed_ptr.Id] = job_completed_ptr
			delete(servers_x_tasks_x_jobs[job_completed_ptr.Server_id][job_completed_ptr.Task_id], job_completed_ptr.Id)
			if len(servers_x_tasks_x_jobs[job_completed_ptr.Server_id][job_completed_ptr.Task_id]) == 0 {
				delete(servers_x_tasks_x_jobs[job_completed_ptr.Server_id], job_completed_ptr.Task_id)
			}
			// record the keys got by the mapper and memorize that such server has those keys' results
			for _, v := range job_completed_ptr.Keys {
				keys_x_servers := task_ptr.Keys_x_servers
				value, ok := keys_x_servers.Get(v)
				if !ok {
					value = make(map[string]*Server)
					keys_x_servers.Set(v, value)
				}
				server := working_mapper_hashmap[job_completed_ptr.Server_id]

				//server_light := Server{server.Id, server.Ip, server.Port, server.Last_heartbeat, server.Role}
				value.(map[string]*Server)[job_completed_ptr.Server_id] = server //&server_light
			}
			InfoLoggerPtr.Println("Mapper job", job_completed_ptr.Id, "completed, task", job_completed_ptr.Task_id, ".")
			// checking if there are jobs to be assigned...
			if len(servers_x_tasks_x_jobs[job_completed_ptr.Server_id]) == 0 { // worker is idle
				for loop := true; loop; {
					select {
					case job_ptr := <-job_channel:
						if _, present := task_hashmap.Get(job_ptr.Task_id); !present { continue }
						assign_job_mapper(working_mapper_hashmap[job_completed_ptr.Server_id], job_ptr, working_mapper_hashmap, servers_x_tasks_x_jobs)
					default:
						idle_mapper_hashmap[job_completed_ptr.Server_id] = working_mapper_hashmap[job_completed_ptr.Server_id]
						// TODO move it outside the loop
						delete(working_mapper_hashmap, job_completed_ptr.Server_id)
					}
					loop = false
				}
				// the task has been completed
				if len(task_ptr.Jobs) == 0 {
					if task_ptr.Send_time == 0 { // if the task has not been sent to the reducer phase already
						InfoLoggerPtr.Println("Map task", job_completed_ptr.Task_id, "completed.")
						task_ptr.Send_time = time.Now().Unix()
						// preparing the task for the reducer (we don want them to refer to the same data structure, we aren't using mutexes)
						task_ptr2 := &Task{
							Id:task_ptr.Id,
							Origin_id:task_ptr.Origin_id,
							Send_time:task_ptr.Send_time,
							Resource_link:task_ptr.Resource_link,
							Mappers_amount:task_ptr.Mappers_amount,
							Margin:task_ptr.Margin,
							Separate_entries:task_ptr.Separate_entries,
							Separate_properties:task_ptr.Separate_properties,
							Properties_amount:task_ptr.Properties_amount,
							Initialization_algorithm:task_ptr.Initialization_algorithm,
							Map_algorithm:task_ptr.Map_algorithm,
							Map_algorithm_parameters:task_ptr.Map_algorithm_parameters,
							Reducers_amount:task_ptr.Reducers_amount,
							Reduce_algorithm:task_ptr.Reduce_algorithm,
							Reduce_algorithm_parameters:task_ptr.Reduce_algorithm_parameters,
							Join_algorithm:task_ptr.Join_algorithm,
							Join_algorithm_parameters:task_ptr.Join_algorithm_parameters,
							Iteration_algorithm:task_ptr.Iteration_algorithm,
							Iteration_algorithm_parameters:task_ptr.Iteration_algorithm_parameters,
							Keys_x_servers:orderedmap.NewOrderedMap(),
							Keys_x_servers_version:0,
							Jobs:make(map[string]*Job),
							Jobs_done:make(map[string]*Job),
						}
						// just copying, same reason as before
						for node := task_ptr.Keys_x_servers.Front(); node != nil; node = node.Next(){
							v := node.Value
							k := node.Key.(string)
							//InfoLoggerPtr.Println("chiave", k, "value", v)
							task_ptr2.Keys_x_servers.Set(k, v)
						}
						// sending the task to the reducer scheduler
						select {
						case Task_reducer_channel <- task_ptr2:
							select {
							case New_task_reducer_event_channel <- struct{}{}:
							default:
								ErrorLoggerPtr.Fatal("Task channel is full")
							}
						default:
							ErrorLoggerPtr.Fatal("Task reduce channel is full")
						}
					} else { // if the task was already sent to the reducer, then send only the updated keys_x_servers
						InfoLoggerPtr.Println("Task mapper re - completed, job", job_completed_ptr.Id, "task", job_completed_ptr.Task_id, ".")
						keys_x_servers := orderedmap.NewOrderedMap()

						for node := task_ptr.Keys_x_servers.Front(); node != nil; node = node.Next(){
							v := node.Value
							k := node.Key.(string)
							keys_x_servers.Set(k, v)
						}

						select {
						case probable_reducer_error_channel <- &map_to_reduce_error{task_ptr.Id, keys_x_servers}:
						default:
							ErrorLoggerPtr.Fatal("Probable_reducer_error_channel is full.")
						}
					}

					if task_hashmap.Len() < MAX_MAP_TASKS && len(Task_mapper_channel) > 0 { // if the curent task finished and theres a task
						select {
						case New_task_mapper_event_channel <-struct{}{}:
						default:
							ErrorLoggerPtr.Fatal("New_task_mapper_event_channel full.")
						}
					}
				}
			}
		// a task has been fully completed, so we can delete its data
		case task_id := <-task_reducer_completed_channel:
			task_hashmap.Delete(task_id)

			servers_map := make(map[string]*Server)
			servers_x_tasks := make(map[string]map[string]struct{})
			for server_id, task_map := range servers_x_tasks_x_jobs_done {
				if server_ptr, ok := idle_mapper_hashmap[server_id]; ok {
					servers_map[server_id] = server_ptr
				} else if server_ptr, ok := working_mapper_hashmap[server_id]; ok {
					servers_map[server_id] = server_ptr
				}
				if _, ok := servers_map[server_id]; ok {
					servers_x_tasks[server_id] = make(map[string]struct{})
					for task_id, _ := range task_map {
						if _, ok2 := task_hashmap.Get(task_id); ok2 {
							continue
						}
						servers_x_tasks[server_id][task_id] = struct{}{}
					}
				}

			}
			go clean_mappers_goroutine(servers_x_tasks, servers_map)

			if task_hashmap.Len() < MAX_MAP_TASKS && len(Task_mapper_channel) > 0 { // if the curent task finished and theres a task
				select {
				case New_task_mapper_event_channel <-struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("New_task_mapper_event_channel full.")
				}
			}

		case <-New_task_mapper_event_channel:
			// checking how many tasks are running, as we said no more than 2 tasks can be managed at once in the scheduler mapper
			// one waiting for finishing the reduce phase
			// one processing in the maps phase
			process_new_task := false
			latest_task_map_el := task_hashmap.Front()
			var latest_task_ptr *Task
			if latest_task_map_el != nil {
				latest_task_ptr = latest_task_map_el.Value.(*Task)
				if len(latest_task_ptr.Jobs) == 0 { // current job finished
					if task_hashmap.Len() < MAX_MAP_TASKS { // 2
						process_new_task = true
					} else {
						InfoLoggerPtr.Println("Waiting one reduce task to finish.")
					}
				}
			} else { process_new_task = true }
			if process_new_task {
				select {
				case task_ptr := <-Task_mapper_channel:

					// cleaning completed tasks
					for _, task_map := range servers_x_tasks_x_jobs_done {
						for task_id, _ := range task_map {
							if _, ok := task_hashmap.Get(task_id); !ok {
								delete(task_map, task_id)
							}
						}
					}

					// preparing the new task, creating and assigning the jobs
					task_ptr.Id = strconv.FormatInt(int64(task_counter), 10)
					task_counter += 1
					task_hashmap.Set(task_ptr.Id, task_ptr)
					InfoLoggerPtr.Println("Scheduling mapper task:", task_ptr.Id)
					resource_size := Get_file_size(task_ptr.Resource_link)
					if int64(len(idle_mapper_hashmap)) > MaxInt { ErrorLoggerPtr.Println("Overflow!!") }
					mappers_amount := MinOf_int32(task_ptr.Mappers_amount, int32(len(idle_mapper_hashmap)))
					if mappers_amount == 0 { mappers_amount = 1 }
					if float64(resource_size) / float64(mappers_amount) > math.MaxInt64 { ErrorLoggerPtr.Println("Overflow!!") }
					slice_size := int64(math.Abs(float64(resource_size) / float64(mappers_amount)))
					jobs := make([]*Job, mappers_amount)
					{
						var i int32 = 0
						alg := make(map[string]string)
						alg_par := make(map[string]interface{})
						alg["map"] = task_ptr.Map_algorithm
						alg["join"] = task_ptr.Join_algorithm
						alg_par["map"] = task_ptr.Map_algorithm_parameters
						alg_par["join"] = task_ptr.Join_algorithm_parameters
						for ; i < mappers_amount; i += 1 {
							begin := int64(i) * slice_size
							end := (int64(i) + 1) * slice_size
							if i == 0 { begin = 0 }
							if i == mappers_amount { end = resource_size }
							jobs[i] = &Job{
								Id: strconv.FormatInt(int64(i), 10),
								Task_id: task_ptr.Id,
								Origin_task_id: task_ptr.Origin_id,
								Server_id: "",
								Resource_link: task_ptr.Resource_link,
								Begin: begin,
								End: end,
								Margin: task_ptr.Margin,
								Separate_entries: task_ptr.Separate_entries,
								Separate_properties: task_ptr.Separate_properties,
								Properties_amount: task_ptr.Properties_amount,
								Algorithm: alg,
								Algorithm_parameters: alg_par,
								Result: nil,
								Keys: nil,
								Keys_x_servers: nil,
								Keys_x_servers_version: 0,
								Delete: false,
							}

							task_ptr.Jobs[strconv.FormatInt(int64(i), 10)] = jobs[i]
						}
					}
					// if there are free workers assign the jobs otherwise add them to the job queue
					if len(idle_mapper_hashmap) > 0 {
						var i int32 = 0
						for _, server_ptr := range idle_mapper_hashmap {
							if i >= mappers_amount { break }
							assign_job_mapper(server_ptr, jobs[i], working_mapper_hashmap, servers_x_tasks_x_jobs)
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

// check if the result of this iteration is the same as the previous one
func iteration_algorithm_clustering_deep_equal(a map[string]interface{}, b map[string]interface{}, max_diff int) (bool) {

	if len(a) != len(b) {
		WarningLoggerPtr.Println("Lengths are supposed to be equal! a", len(a), "b", len(b))
		return false
	}


	for i, v_o := range a {
		v := v_o.([]float64)
		b_i := b[i].([]float64)
		if len(v) != len(b_i) {
			WarningLoggerPtr.Println("Lengths are supposed to be equal!")
			return false
		}

		for i2, v2 := range v {
			diff := math.Abs(v2 - b_i[i2])
			// A max_diff (percentage) is needed because the welford one pass (with a different order of samples) will return a slightly different result
			if diff > (math.Abs(v2) / 100) * float64(max_diff) && diff > (math.Abs(b_i[i2]) / 100) * float64(max_diff) {
				return false
			}

		}

	}

	return true
}
// "custom" function to understand if we need to continue the iteration
func iteration_algorithm_clustering(task_ptr *Task, new_task_ptr_ptr **Task, keys_x_values map[string]interface{}) (bool) {

	if len(task_ptr.Iteration_algorithm_parameters.([]interface{})) == 1 {
		// if it is the first run then add the result to the algorithm parameters slice so that we can check if we have found a fixpoint in the next iteration
		task_ptr.Iteration_algorithm_parameters = append(task_ptr.Iteration_algorithm_parameters.([]interface{}), keys_x_values)
	} else {
		old_keys_x_values := task_ptr.Iteration_algorithm_parameters.([]interface{})[1].(map[string]interface{})
		// check if the old iteration's result is equal to the current one
		if iteration_algorithm_clustering_deep_equal(old_keys_x_values, keys_x_values, task_ptr.Iteration_algorithm_parameters.([]interface{})[0].(int)) {
			InfoLoggerPtr.Println("Fixpoint found, iteration concluded")
			// printing the fix point
			result_string := ""
			for key, key_value_o := range keys_x_values {
				key_value := key_value_o.([]float64)
				result_string += fmt.Sprintln("key", key, "value", key_value)

				//InfoLoggerPtr.Printf(result_string)

			}

			result_struct := &Task_result{task_ptr.Id, task_ptr.Origin_id, result_string}
			// pushing the results to a channel to let the user read them
			select {
			case Result_for_JRPC_channel<-result_struct:
			default:
				// TODO the for loop is missing here!
				WarningLoggerPtr.Println("Result for JRPC channel is full, popping one element.")
				select {
				case <-Result_for_JRPC_channel:
				default:
				}
			}
			return true
		}
		// updating the keys_x_values to check equality in the end of the next iteration
		task_ptr.Iteration_algorithm_parameters.([]interface{})[1] = keys_x_values
	}
	// updating the centroids for the next iteration
	for index_string, value := range keys_x_values {
		index, over := strconv.Atoi(index_string)
		if over != nil { ErrorLoggerPtr.Println("Overflow!!") }
		task_ptr.Map_algorithm_parameters.([]interface{})[index + 1] = value.([]float64)
	}

	if task_ptr.Origin_id == "-1" { task_ptr.Origin_id = task_ptr.Id }

	*new_task_ptr_ptr = &Task{
		Id:"-1",
		Origin_id:task_ptr.Origin_id,
		Send_time:0,
		Resource_link:task_ptr.Resource_link,
		Mappers_amount:task_ptr.Mappers_amount,
		Margin:task_ptr.Margin,
		Separate_entries:task_ptr.Separate_entries,
		Separate_properties:task_ptr.Separate_properties,
		Properties_amount:task_ptr.Properties_amount,
		Initialization_algorithm:task_ptr.Initialization_algorithm,
		Map_algorithm:task_ptr.Map_algorithm,
		Map_algorithm_parameters:task_ptr.Map_algorithm_parameters,
		Reducers_amount:task_ptr.Reducers_amount,
		Reduce_algorithm:task_ptr.Reduce_algorithm,
		Reduce_algorithm_parameters:task_ptr.Reduce_algorithm_parameters,
		Join_algorithm:task_ptr.Join_algorithm,
		Join_algorithm_parameters:task_ptr.Join_algorithm_parameters,
		Iteration_algorithm:task_ptr.Iteration_algorithm,
		Iteration_algorithm_parameters:task_ptr.Iteration_algorithm_parameters,
		Keys_x_servers:orderedmap.NewOrderedMap(),
		Keys_x_servers_version:0,
		Jobs:make(map[string]*Job),
		Jobs_done:make(map[string]*Job),
	}
	InfoLoggerPtr.Println("Fixpoint not found yet, new_task created")

	// printing iteration's result
	for index, index_value_o := range task_ptr.Map_algorithm_parameters.([]interface{})[1:] {
		index_value := index_value_o.([]float64)
		InfoLoggerPtr.Println("key", index, "value", index_value)
	}
	return false
}
// checking if the iteration concluded, otherwise a new task will be created and sent to the mapper scheduler
func iteration_manager(task_ptr *Task, keys_x_values map[string]interface{}) {
	var new_task_ptr *Task
	res, err := Call("iteration_algorithm_" + task_ptr.Iteration_algorithm, stub_storage, task_ptr, &new_task_ptr, keys_x_values)
	if err != nil { ErrorLoggerPtr.Fatal("Error calling iteration_algorithm:", err) }

	if !res.(bool) {
		select {
		case Task_mapper_channel <- new_task_ptr:
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
}

// Just sending the job to the reducer and update the data structures
func assign_job_reducer(server_ptr *Server, job_ptr *Job, working_reducer_hashmap map[string]*Server, servers_x_tasks_x_jobs map[string]map[string]map[string]*Job) {

	job_ptr.Server_id = server_ptr.Id
	working_reducer_hashmap[server_ptr.Id] = server_ptr
	InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to reducer", job_ptr.Server_id)
	{
		job_map, ok := servers_x_tasks_x_jobs[server_ptr.Id][job_ptr.Task_id]
		if !ok {
			job_map = make(map[string]*Job)
			servers_x_tasks_x_jobs[server_ptr.Id][job_ptr.Task_id] = job_map
		}
		job_map[job_ptr.Id] = job_ptr
	}
	keys := make([]string, 0)
	for key, _ := range job_ptr.Keys_x_servers {
		keys = append(keys, key)
	}
	go Rpc_job_goroutine(server_ptr, job_ptr, "Reducer_handler.Send_job",
	"Sent reducer job " + job_ptr.Id + " : " + fmt.Sprint(keys)  + " task " + job_ptr.Task_id,
			3, EXPIRE_TIME, false)

}

// notify mappers that the task ended so that they can clean delete its data
func clean_mappers_goroutine(servers_x_tasks map[string]map[string]struct{}, servers_map map[string]*Server) {
	for server_id, task_map := range servers_x_tasks {
		if len(task_map) == 0 { continue }
		req := &Request{
			Sender: nil,
			Receiver:nil,
			Tries: 3,
			Time: time.Now(),
			Body:task_map,
		}

		server_ptr := servers_map[server_id]
		go Rpc_request_goroutine(server_ptr, req, "Mapper_handler.Task_completed",
			"Sent completed task notification " + fmt.Sprint(task_map) +  " to the mapper " + server_ptr.Id,
			1, EXPIRE_TIME, false)
	}
}

func scheduler_reducer_goroutine() {
	InfoLoggerPtr.Println("Scheduler_reducer_goroutine started.")

	state := IDLE
	job_channel := make(chan *Job, 1000)
	keys_x_servers_update_channel := make(chan *map_to_reduce_error, 1000)
	idle_reducer_hashmap := make(map[string]*Server)
	working_reducer_hashmap := make(map[string]*Server)
	task_hashmap := orderedmap.NewOrderedMap()
	servers_x_tasks_x_jobs := make(map[string]map[string]map[string]*Job)
	servers_x_tasks_x_jobs_done := make(map[string]map[string]map[string]*Job)
	keys_x_values := make(map[string]interface{})

	for {
		select {
		// mapper scheduler will notify the reducer scheduler through this channel if a mapper crashed
		case error_ptr := <-probable_reducer_error_channel:
			// just a notification
			if error_ptr.keys_x_servers == nil && state == BUSY { // have to wait till mapper will produce the new keys_x_servers
				if el := task_hashmap.Front(); el != nil {
					if el.Key.(string) == error_ptr.task_id {
						WarningLoggerPtr.Println("Reducer state changing to WAIT because a mapper crashed", error_ptr.task_id)
						state = WAIT // we won't assign new jobs till we are in this state (we do not want to send jobs with an old kays_x_values)
					}
				}
			// otherwise it means the new keys_x_servers is ready and we have to update it
			} else {
				if state == WAIT { state = BUSY }
				task_ptr_o, ok := task_hashmap.Get(error_ptr.task_id)
				if ok {
					task_ptr := task_ptr_o.(*Task)
					task_ptr.Keys_x_servers = error_ptr.keys_x_servers
					task_ptr.Keys_x_servers_version += 1
					// if there are idle servers then we can assign a job
					for _, server_ptr := range idle_reducer_hashmap {
						loop_break := false
						select {
						case job_ptr := <-job_channel:
							// if the job has an old version of keys_x_values then we have to update it
							if job_ptr.Keys_x_servers_version != task_ptr.Keys_x_servers_version {
								for key, key_content := range job_ptr.Keys_x_servers {
									servers_map, present := task_ptr.Keys_x_servers.Get(key)
									if !present { ErrorLoggerPtr.Fatal("Key not found.") }
									key_content = servers_map.(map[string]*Server)

									// compiler thinks i'm not using this variable... ?!?
									key_content = key_content
								}
							}
							assign_job_reducer(server_ptr, job_ptr, working_reducer_hashmap, servers_x_tasks_x_jobs)
						default:
							loop_break = true
						}
						if loop_break { break }
					}
				} else {
					select {
					// if the task hasn't tasted yer then we have to update the keys_x_servers in the case which manage the new task
					case keys_x_servers_update_channel <- error_ptr:
					default:
						ErrorLoggerPtr.Println("keys_x_servers_update_channel is full.")
					}
				}
			}
		case rem_reducer_ptr := <-rem_reducer_channel:
			InfoLoggerPtr.Println("Reducer", rem_reducer_ptr.Id, "ip", rem_reducer_ptr.Ip, "port", rem_reducer_ptr.Port, "is being added.")
			// removing a reducer
			delete(idle_reducer_hashmap, rem_reducer_ptr.Id)
			// data structure to store the new keys_x_servers variable

			/*if _, ok := idle_reducer_hashmap[rem_reducer_ptr.Id]; ok {
				delete(idle_reducer_hashmap, rem_reducer_ptr.Id)
			} else if server, ok := working_reducer_hashmap[rem_reducer_ptr.Id]; ok {*/
			for el := task_hashmap.Back(); el != nil; el = el.Prev() { // checking each task for jobs that have to be rescheduled (in the current implementation there can be only one task)
				current_task_ptr := el.Value.(*Task).Id
				jobs := servers_x_tasks_x_jobs[rem_reducer_ptr.Id][current_task_ptr]
				for _, job_ptr := range jobs {
					// if there is some idle mapper then assign the job, otherwise add it to the to-do job channel (kind of a queue)
					if state != WAIT && len(idle_reducer_hashmap) > 0 {
						for server_id, server_ptr := range idle_reducer_hashmap {
							delete(idle_reducer_hashmap, server_id)
							assign_job_reducer(server_ptr, job_ptr, working_reducer_hashmap, servers_x_tasks_x_jobs)
							break
						}
					} else {
						select {
						case job_channel <- job_ptr:
							InfoLoggerPtr.Println("Reduce job", job_ptr.Id, "rescheduled.")
						default:
							ErrorLoggerPtr.Fatal("job_channel queue full")
						}
					}
				}
			}
			delete(working_reducer_hashmap, rem_reducer_ptr.Id)
			delete(servers_x_tasks_x_jobs, rem_reducer_ptr.Id)
			delete(servers_x_tasks_x_jobs_done, rem_reducer_ptr.Id)

		// adding a new reducer
		case add_reducer_ptr := <-add_reducer_channel:
			InfoLoggerPtr.Println("Reducer", add_reducer_ptr.Id, "ip", add_reducer_ptr.Ip, "port", add_reducer_ptr.Port, "is being added")
			// preparing data structures
			servers_x_tasks_x_jobs[add_reducer_ptr.Id] = make(map[string]map[string]*Job)
			servers_x_tasks_x_jobs_done[add_reducer_ptr.Id] = make(map[string]map[string]*Job)

			if state != WAIT {
				// we have a new reducer, so let's try to see if there's some job to be processed
				select {
				case job_ptr := <-job_channel:
					assign_job_reducer(add_reducer_ptr, job_ptr, working_reducer_hashmap, servers_x_tasks_x_jobs)
				default:
					idle_reducer_hashmap[add_reducer_ptr.Id] = add_reducer_ptr
				}
			} else {
				idle_reducer_hashmap[add_reducer_ptr.Id] = add_reducer_ptr
			}
		// job has been completed, so we have to check if perhaps the task finished
		case job_completed_ptr := <-Job_reducer_completed_channel:

			task_ptr_o, present := task_hashmap.Get(job_completed_ptr.Task_id)
			if !present { break } // just ignore if we do not know the task
			task_ptr := task_ptr_o.(*Task)
			// moving the job form the to-do jobs data structure to the done jobs data structure
			{
				job_map, ok := servers_x_tasks_x_jobs_done[job_completed_ptr.Server_id][job_completed_ptr.Task_id]
				if !ok {
					job_map = make(map[string]*Job)
					servers_x_tasks_x_jobs_done[job_completed_ptr.Server_id][job_completed_ptr.Task_id] = job_map
				}
				job_map[job_completed_ptr.Id] = job_completed_ptr
			}

			// combining the results from the reducers
			for key, key_value := range job_completed_ptr.Result {
				_, ok := keys_x_values[key]
				if !ok {
					keys_x_values[key] = key_value
				} else {
					_, err := Call("Join_algorithm_" + task_ptr.Join_algorithm, stub_storage,
						keys_x_values[key], key_value, task_ptr.Join_algorithm_parameters)
					if err != nil { ErrorLoggerPtr.Fatal("Error calling mapper_algorithm:", err) }
				}
			}
			// moving the job form the to-do jobs data structure to the done jobs data structure
			delete(task_ptr.Jobs, job_completed_ptr.Id)
			task_ptr.Jobs_done[job_completed_ptr.Id] = job_completed_ptr
			delete(servers_x_tasks_x_jobs[job_completed_ptr.Server_id][job_completed_ptr.Task_id], job_completed_ptr.Id)
			if len(servers_x_tasks_x_jobs[job_completed_ptr.Server_id][job_completed_ptr.Task_id]) == 0 {
				delete(servers_x_tasks_x_jobs[job_completed_ptr.Server_id], job_completed_ptr.Task_id)
			}

			InfoLoggerPtr.Println("Reducer job", job_completed_ptr.Id, "completed, task", job_completed_ptr.Task_id, ".")

			// checking if there are jobs to be assigned...
			if len(servers_x_tasks_x_jobs[job_completed_ptr.Server_id]) == 0 { // worker is idle
				if state != WAIT {
					select {
					case job_ptr := <-job_channel:
						assign_job_reducer(working_reducer_hashmap[job_completed_ptr.Server_id], job_ptr, working_reducer_hashmap, servers_x_tasks_x_jobs)
					default:
						idle_reducer_hashmap[job_completed_ptr.Server_id] = working_reducer_hashmap[job_completed_ptr.Server_id]
						// TODO move it outside the loop
						delete(working_reducer_hashmap, job_completed_ptr.Server_id)
					}
				} else {
					idle_reducer_hashmap[job_completed_ptr.Server_id] = working_reducer_hashmap[job_completed_ptr.Server_id]
					delete(working_reducer_hashmap, job_completed_ptr.Server_id)
				}
				if len(task_ptr.Jobs) == 0 {
				// last job of the task, thus the task ended
					InfoLoggerPtr.Println("Reducer task", job_completed_ptr.Task_id, "completed.")
					state = IDLE
					select {
					case task_reducer_completed_channel <-job_completed_ptr.Task_id:
					default:
						ErrorLoggerPtr.Fatal("Task_reducer_completed full.")
					}
					task_ptr, found := task_hashmap.Get(job_completed_ptr.Task_id)
					if !found { ErrorLoggerPtr.Println("Task not found") }
					// check if we are done or a new iteration is needed
					go iteration_manager(task_ptr.(*Task), keys_x_values)
					keys_x_values = make(map[string]interface{})
					task_hashmap.Delete(job_completed_ptr.Task_id)
				}
				if state == IDLE && len(Task_reducer_channel) > 0 { // if the curent task finished and theres a task
					select {
					case New_task_reducer_event_channel <-struct{}{}:
					default:
						ErrorLoggerPtr.Fatal("New_task_reducer_event_channel full.")
					}
				}
			}
		case <-New_task_reducer_event_channel:
			if task_hashmap.Len() == 0 { // 1 task running each time
				select {
				case task_ptr := <-Task_reducer_channel:
					// check for keys_x_servers update
					for loop := true; loop; {
						select {
						case error_ptr := <-keys_x_servers_update_channel:
							if task_ptr.Id == error_ptr.task_id {
								task_ptr.Keys_x_servers = error_ptr.keys_x_servers
								task_ptr.Keys_x_servers_version += 1
							}
						default:
							loop = false
						}
					}


					// cleaning completed tasks
					for _, task_map := range servers_x_tasks_x_jobs_done {
						for task_id, _ := range task_map {
							if _, ok := task_hashmap.Get(task_id); !ok {
								delete(task_map, task_id)
							}
						}
					}

					// preparing the new task, creating and assigning the jobs
					task_hashmap.Set(task_ptr.Id, task_ptr)
					state = BUSY
					InfoLoggerPtr.Println("Scheduling reducer task:", task_ptr.Id)
					if task_ptr.Keys_x_servers.Len() > math.MaxInt32 { ErrorLoggerPtr.Println("Overflow!!") }
					keys_amount := int32(task_ptr.Keys_x_servers.Len())
					if len(idle_reducer_hashmap) > math.MaxInt32 { ErrorLoggerPtr.Println("Overflow!!") }
					reducers_amount := MinOf_int32(task_ptr.Reducers_amount, int32(len(idle_reducer_hashmap)), int32(task_ptr.Keys_x_servers.Len()))
					slice_size := 1
					if reducers_amount == 0 { reducers_amount = 1 }
					var slice_reminder int32 = 0
					// distributing the keys over the reducers
					if keys_amount > reducers_amount {
						if Check_float64_to_int_overflow(math.Abs(float64(keys_amount) / float64(reducers_amount))) {
							ErrorLoggerPtr.Println("Overflow!!")
						}
						slice_size = int(math.Abs(float64(keys_amount) / float64(reducers_amount)))
						slice_reminder = keys_amount % reducers_amount
					}
					if reducers_amount == 0 { reducers_amount = 1 }

					jobs := make([]*Job, reducers_amount)
					el := task_ptr.Keys_x_servers.Front()
					{
						//InfoLoggerPtr.Println("slice size:", slice_size, "reminder:", slice_reminder)
						var i int32 = 0

						alg := make(map[string]string)
						alg_par := make(map[string]interface{})
						alg["reduce"] = task_ptr.Reduce_algorithm
						alg["join"] = task_ptr.Join_algorithm
						alg_par["reduce"] = task_ptr.Reduce_algorithm_parameters
						alg_par["join"] = task_ptr.Join_algorithm_parameters

						for ; i < reducers_amount; i += 1 {
							current_slice_size := slice_size
							// distributing the remainder
							if slice_reminder > 0 {
								current_slice_size += 1
								slice_reminder -= 1
							}

							keys := make(map[string]map[string]*Server)
							{
								j := 0
								//InfoLoggerPtr.Println("size", current_slice_size)
								for ; j < current_slice_size && el != nil; j += 1 {
									value := el.Value.(map[string]*Server)
									keys[el.Key.(string)] = value
									//InfoLoggerPtr.Println("key added ", el.Key.(string))
									el = el.Next()
								}
								if j < current_slice_size && el == nil {
									ErrorLoggerPtr.Fatal("There are less keys than expected.")
								}
							}

							jobs[i] = &Job{
								Id: strconv.FormatInt(int64(i), 10),
								Task_id: task_ptr.Id,
								Origin_task_id: task_ptr.Origin_id,
								Server_id: "",
								Resource_link: "",
								Begin: 0,
								End: 0,
								Margin: 0,
								Separate_entries: task_ptr.Separate_entries,
								Separate_properties: task_ptr.Separate_properties,
								Properties_amount: task_ptr.Properties_amount,
								Algorithm: alg,
								Algorithm_parameters: alg_par,
								Result: nil,
								Keys: nil,
								Keys_x_servers: keys,
								Keys_x_servers_version: 0,
								Delete: false,
							}

							task_ptr.Jobs[strconv.FormatInt(int64(i), 10)] = jobs[i]
						}
					}
					// if there are free workers assign the jobs otherwise add them to the job queue
					if len(idle_reducer_hashmap) > 0 {
						i := 0
						for _, server_ptr := range idle_reducer_hashmap {

							assign_job_reducer(server_ptr, jobs[i], working_reducer_hashmap, servers_x_tasks_x_jobs)
							i += 1
							if i > len(jobs) - 1 { break }
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


func json_rpc_manager_goroutine() {
	InfoLoggerPtr.Println("Starting json rpc goroutine")
	s := rpc.NewServer()
	s.RegisterCodec(json.NewCodec(), "application/json")
	s.RegisterService(new(JSONServer), "")

	r := mux.NewRouter()
	r.Handle("/rpc", s)
	ErrorLoggerPtr.Fatal(http.ListenAndServe(":" + MASTER_PORT_JSON_RPC, r))
}

func Master_main() {
	// initializing status, 0 mappers, 0 reducers
	Status_ptr = &Status{0, 0}
	// seeding the RNG
	rand.Seed(time.Now().UnixNano())
	// registering types for interface{}
	gob.Register([]interface{}(nil))
	gob.Register(map[string]string(nil))

	// data structure for calling functions by its name in a string
	stub_storage = map[string]interface{}{
		"iteration_algorithm_clustering": iteration_algorithm_clustering,
		"initialization_algorithm_clustering": initialization_algorithm_clustering,
		"Join_algorithm_clustering": Join_algorithm_clustering,
	}

	// creating channel for communicating the heartbeat
	// to the goroutine heartbeat manager
	Heartbeat_channel = make(chan *Server, 1000)


	// creating channel for communicating the connected
	// and disconnected workers to the scheduler
	add_mapper_channel = make(chan *Server, 1000)
	rem_mapper_channel = make(chan *Server, 1000)

	// creating channel for communicating the connected
	// and disconnected workers to the scheduler
	add_reducer_channel = make(chan *Server, 1000)
	rem_reducer_channel = make(chan *Server, 1000)

	//creating channel for communicating ended jobs
	Job_mapper_completed_channel = make(chan *Job, 1000)

	//creating channel for communicating new task event
	New_task_mapper_event_channel = make(chan struct{}, 1000)

	//creating channel for communicating new task event
	New_task_reducer_event_channel = make(chan struct{}, 1000)

	//creating channel for communicating new task
	Task_mapper_channel = make(chan *Task, 1000)

	//creating channel for communicating new task
	Task_reducer_channel = make(chan *Task, 1000)

	//creating channel for communicating ended jobs
	Job_reducer_completed_channel = make(chan *Job, 1000)

	//creating channel for communicating to the mapper scheduler that the task is finished
	task_reducer_completed_channel = make(chan string, 1000)

	//creating channel for communicating to te reducer scheduler that a mapper crashed and errors could occut
	probable_reducer_error_channel = make(chan *map_to_reduce_error, 1000)

	//creating channel for communicating new tasks from the users
	Task_from_JRPC_channel = make(chan *[]interface{}, 1000)

	//creating channel for communicating results to the users
	Result_for_JRPC_channel = make(chan *Task_result, 1000)

	go scheduler_reducer_goroutine()
	go scheduler_mapper_goroutine()
	go heartbeat_goroutine()
	go task_injector_goroutine()

	master_handler := new(Master_handler)

	// register Master_handler as RPC interface
	go_rpc.Register(master_handler)

	go json_rpc_manager_goroutine()

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
		go go_rpc.ServeConn(conn)
	}

}
