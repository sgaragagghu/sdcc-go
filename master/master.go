package master

import (
	. "../share"
	. "../rpc_master"
//	. "../rpc_mapper"
	"net"
	"net/rpc"
	"reflect"
	"time"
	"encoding/gob"
	//"container/list"
	"math"
	"math/rand"
	"strconv"
	"bytes"
	"bufio"

	"github.com/elliotchance/orderedmap"
)


type map_to_reduce_error struct {
	task_id string
	keys_x_servers *orderedmap.OrderedMap
}

type task struct {
	id int32
	origin_id int32
	send_time int64
	resource_link string
	mappers_amount int32
	margin int8
	separate_entries byte
	separate_properties byte
	properties_amount int8
	initialization_algorithm string
	map_algorithm string
	map_algorithm_parameters interface{}
//	shuffle_algorithm string
//	order_algorithm string
	reducers_amount int32
	reduce_algorithm string
	reduce_algorithm_parameters interface{}
	iteration_algorithm string
	iteration_algorithm_parameters interface{}
	keys_x_servers *orderedmap.OrderedMap
	keys_x_servers_version int8
	jobs map[string]*Job
	jobs_done map[string]*Job
}

var (
	// TODO both to be moved to the rpc file (if i'll make it)
	New_task_mapper_event_channel chan struct{}
	New_task_reducer_event_channel chan struct{}
	Task_mapper_channel chan *task
	Task_reducer_channel chan *task

	add_mapper_channel chan *Server
	rem_mapper_channel chan *Server
	add_reducer_channel chan *Server
	rem_reducer_channel chan *Server

	probable_reducer_error_channel chan *map_to_reduce_error
	task_reducer_completed_channel chan string

	stub_storage StubMapping
)

func initialization_algorithm_clustering(task_ptr *task) (ret bool) {
	ret = false
	resource_size := Get_file_size(task_ptr.resource_link)
	offsets := make([][]float64, task_ptr.map_algorithm_parameters.([]interface{})[0].(int))

	for i, _ := range offsets {
		offsets[i] = make ([]float64, 2)
		download_size := int(math.Abs(((float64(resource_size) / float64(task_ptr.mappers_amount)) / 100) * float64(task_ptr.margin))) // TODO check overflow
		// TODO check possible overflow
		offset := rand.Intn(int(resource_size) - download_size) // TODO check seed

		load_ptr := Http_download(task_ptr.resource_link, int64(offset), int64(offset + download_size))

		actual_begin, err := Get_actual_begin(load_ptr, task_ptr.separate_entries)
		if err != nil { ErrorLoggerPtr.Fatal("get_actual_begin error:", err) }

		actual_end, err := Get_actual_end(load_ptr, task_ptr.separate_entries, actual_begin + 1)
		if err != nil { ErrorLoggerPtr.Fatal("get_actual_end error:", err) }

		//InfoLoggerPtr.Println("Actual begin:", actual_begin, "actual end:", actual_end)

		if actual_begin == actual_end { ErrorLoggerPtr.Fatal("Unexpected error") }


		reader := bytes.NewReader((*load_ptr)[actual_begin:actual_end + 1])
		buffered_read := bufio.NewReader(reader)
		Parser_simple(&(offsets[i]), buffered_read, task_ptr.separate_properties, task_ptr.separate_entries)
	}

	InfoLoggerPtr.Println("Seed:")
	for i, v := range offsets {
		task_ptr.map_algorithm_parameters.([]interface{})[i + 1] = v
		InfoLoggerPtr.Println(v)
	}

	ret = true
	return
}

func task_injector_goroutine() { // TODO make a jsonrpc interface to send tasks from a browser or curl 

	time.Sleep(60 * SECOND)

	parameters := make([]interface{}, 5)
	parameters[0] = 4 // k
	parameters[1] = []float64{0, 0} // u_0
	parameters[2] = []float64{1, 1} // u_1
	parameters[3] = []float64{2, 2} // u_2
	parameters[4] = []float64{3, 3} // u_3


	iteration_parameters := make([]interface{}, 1)
	iteration_parameters[0] =  2 // max_diff (percentage)

	task_ptr := &task{-1, -1, 0, "https://raw.githubusercontent.com/sgaragagghu/sdcc-clustering-datasets/master/sdcc/2d-4c.csv", 2, 10,
		'\n', ',', 2, "clustering", "clustering", parameters, 1, "clustering", nil, "clustering", iteration_parameters, orderedmap.NewOrderedMap(), 0,
		make(map[string]*Job), make(map[string]*Job)}

	// TODO check error and return...
	/*_, _ := */Call("initialization_algorithm_" + task_ptr.initialization_algorithm, stub_storage, task_ptr)

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
				if server_temp_ptr.Role == MAPPER {
					select {
					case add_mapper_channel <-server_temp_ptr:
					default:
						ErrorLoggerPtr.Fatal("Add_mapper_channel is full")
					}
				} else if server_temp_ptr.Role == REDUCER {
					select {
					case add_reducer_channel <-server_temp_ptr:
					default:
						ErrorLoggerPtr.Fatal("Add_reducer_channel is full")
					}
				} else { ErrorLoggerPtr.Fatal("Unexpected kinf of role:", server_temp_ptr.Role) }
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
					if server_temp_ptr.Role == MAPPER {
						select {
						case rem_mapper_channel <-server_temp_ptr:
						default:
							ErrorLoggerPtr.Fatal("Rem_mapper_channel is full")
						}
					} else if server_temp_ptr.Role == REDUCER {

						select {
						case rem_reducer_channel <-server_temp_ptr:
						default:
							ErrorLoggerPtr.Fatal("Rem_reducer_channel is full")
						}
					}
				}
			}
		}
	}
}

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
		"Sent mapper job " + job_ptr.Id + " task " + job_ptr.Task_id,
		3, EXPIRE_TIME, false)
}

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
			delete(idle_mapper_hashmap, rem_mapper_ptr.Id)
/*
			if _, ok := idle_mapper_hashmap[rem_mapper_ptr.Id]; ok {
				delete(idle_mapper_hashmap, rem_mapper_ptr.Id)
			} else if _, ok := working_mapper_hashmap[rem_mapper_ptr.Id]; ok {*/
			probable_reducer_task_error := make(map[string]struct{})
			for el := task_hashmap.Back(); el != nil; el = el.Prev() {
				current_task_ptr := strconv.FormatInt(int64(el.Value.(*task).id), 10)
				task_ptr_o, task_present := task_hashmap.Get(current_task_ptr)
				if !task_present { break }
				task_ptr := task_ptr_o.(*task)
				jobs := servers_x_tasks_x_jobs[rem_mapper_ptr.Id][current_task_ptr]
				for i := 0; i < 2; i += 1 {
					for _, job_ptr := range jobs {
						if i > 0 { // this is the iteration with the already done jobs
							probable_reducer_task_error[job_ptr.Task_id] = struct{}{}
							delete(task_ptr.jobs_done, job_ptr.Id)
							task_ptr.jobs[job_ptr.Id] = job_ptr
							for _, v := range job_ptr.Keys {
								servers, _ := task_ptr.keys_x_servers.Get(v) // TODO check errors
								delete(servers.(map[string]*Server), rem_mapper_ptr.Id)
							}
							WarningLoggerPtr.Println("Mapper", rem_mapper_ptr.Id, "disconnected, previously done job", job_ptr.Id, "task", task_ptr.id,
									"is being rescheduled.")
						} else {
							WarningLoggerPtr.Println("Mapper", rem_mapper_ptr.Id, "disconnected, job", job_ptr.Id, "task", task_ptr.id,
									"is being rescheduled.")
						}
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
								ErrorLoggerPtr.Fatal("job_channel queue full") // TODO handle this case...
							}
						}
					}
					jobs = servers_x_tasks_x_jobs_done[rem_mapper_ptr.Id][current_task_ptr]
				}
			}
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
		case add_mapper_ptr := <-add_mapper_channel:
			InfoLoggerPtr.Println("Mapper", add_mapper_ptr.Id, "ip", add_mapper_ptr.Ip, "port", add_mapper_ptr.Port, "is being added")
			servers_x_tasks_x_jobs[add_mapper_ptr.Id] = make(map[string]map[string]*Job)
			servers_x_tasks_x_jobs_done[add_mapper_ptr.Id] = make(map[string]map[string]*Job)
			for loop := true; loop; {
				select {
				case job_ptr := <-job_channel: // TODO prioritize oldest task's jobs first
					if _, present := task_hashmap.Get(job_ptr.Task_id); !present { continue }
					assign_job_mapper(add_mapper_ptr, job_ptr, working_mapper_hashmap, servers_x_tasks_x_jobs)
				default:
					idle_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
				}
				loop = false
			}
		case job_completed_ptr := <-Job_mapper_completed_channel:
			task_ptr_o, present := task_hashmap.Get(job_completed_ptr.Task_id)
			if !present { break }
			task_ptr := task_ptr_o.(*task)
			{
				job_map, ok := servers_x_tasks_x_jobs_done[job_completed_ptr.Server_id][job_completed_ptr.Task_id]
				if !ok {
					job_map = make(map[string]*Job)
					servers_x_tasks_x_jobs_done[job_completed_ptr.Server_id][job_completed_ptr.Task_id] = job_map
				}
				job_map[job_completed_ptr.Id] = job_completed_ptr
			}

			delete(task_ptr.jobs, job_completed_ptr.Id)
			task_ptr.jobs_done[job_completed_ptr.Id] = job_completed_ptr
			delete(servers_x_tasks_x_jobs[job_completed_ptr.Server_id][job_completed_ptr.Task_id], job_completed_ptr.Id)
			if len(servers_x_tasks_x_jobs[job_completed_ptr.Server_id][job_completed_ptr.Task_id]) == 0 {
				delete(servers_x_tasks_x_jobs[job_completed_ptr.Server_id], job_completed_ptr.Task_id)
			}

			for _, v := range job_completed_ptr.Keys {
				keys_x_servers := task_ptr.keys_x_servers
				value, ok := keys_x_servers.Get(v)
				if !ok {
					value = make(map[string]*Server)
					keys_x_servers.Set(v, value)
				}
				server := working_mapper_hashmap[job_completed_ptr.Server_id]
				// TODO manage it in a way to not waste memory
				server_light := Server{server.Id, server.Ip, server.Port, server.Last_heartbeat, server.Role}
				value.(map[string]*Server)[job_completed_ptr.Server_id] = &server_light
			}
			if len(servers_x_tasks_x_jobs[job_completed_ptr.Server_id]) == 0 {
				for loop := true; loop; {
					select {
					case job_ptr := <-job_channel:
						if _, present := task_hashmap.Get(job_ptr.Task_id); !present { continue }
						assign_job_mapper(working_mapper_hashmap[job_completed_ptr.Server_id], job_ptr, working_mapper_hashmap, servers_x_tasks_x_jobs)
					default:
						idle_mapper_hashmap[job_completed_ptr.Server_id] = working_mapper_hashmap[job_completed_ptr.Server_id]
						delete(working_mapper_hashmap, job_completed_ptr.Server_id)
					}
					loop = false
				}

				if len(task_ptr.jobs) == 0 {
					if task_ptr.send_time == 0 {
						InfoLoggerPtr.Println("Task mapper completed, job", job_completed_ptr.Id, "task", job_completed_ptr.Task_id, ".")
						task_ptr.send_time = time.Now().Unix()
						select {
						case Task_reducer_channel <- task_ptr:
							select {
							case New_task_reducer_event_channel <- struct{}{}:
							default:
								ErrorLoggerPtr.Fatal("Task channel is full")
							}
						default:
							ErrorLoggerPtr.Fatal("Task reduce channel is full")
						}
					} else {
						InfoLoggerPtr.Println("Task mapper re - completed, job", job_completed_ptr.Id, "task", job_completed_ptr.Task_id, ".")

						select {
						case probable_reducer_error_channel <- &map_to_reduce_error{strconv.FormatInt(int64(task_ptr.id), 10), task_ptr.keys_x_servers}:
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

		case task_id := <-task_reducer_completed_channel:
			task_hashmap.Delete(task_id)
			if task_hashmap.Len() < MAX_MAP_TASKS && len(Task_mapper_channel) > 0 { // if the curent task finished and theres a task
				select {
				case New_task_mapper_event_channel <-struct{}{}:
				default:
					ErrorLoggerPtr.Fatal("New_task_mapper_event_channel full.")
				}
			}

		case <-New_task_mapper_event_channel:
			process_new_task := false
			latest_task_map_el := task_hashmap.Front() // TODO check error
			var latest_task_ptr *task
			if latest_task_map_el != nil {
				latest_task_ptr = latest_task_map_el.Value.(*task)
				if len(latest_task_ptr.jobs) == 0 { // current job finished
					if task_hashmap.Len() < MAX_MAP_TASKS {
						process_new_task = true
					} else {
						InfoLoggerPtr.Println("Waiting one reduce task to finish.")
					}
				}
			} else { process_new_task = true }
			if process_new_task {
				select {
				case task_ptr := <-Task_mapper_channel:
					task_ptr.id = task_counter
					task_counter += 1
					task_hashmap.Set(strconv.Itoa(int(task_ptr.id)), task_ptr) // TODO check overflow
					InfoLoggerPtr.Println("Scheduling mapper task:", task_ptr.id)
					resource_size := Get_file_size(task_ptr.resource_link)
					mappers_amount := MinOf_int32(task_ptr.mappers_amount, int32(len(idle_mapper_hashmap))) // TODO check overflow
					if mappers_amount == 0 { mappers_amount = 1 }
					slice_size := int64(math.Abs(float64(resource_size) / float64(mappers_amount))) // TODO math.Abs, does it make sense here ?
					jobs := make([]*Job, mappers_amount)
					{
						var i int32 = 0
						for ; i < mappers_amount; i += 1 {
							begin := int64(i) * slice_size
							end := (int64(i) + 1) * slice_size
							if i == 0 { begin = 0 }
							if i == mappers_amount { end = resource_size }
							jobs[i] = &Job{strconv.FormatInt(int64(i), 10), strconv.FormatInt(int64(task_ptr.id), 10),
								strconv.FormatInt(int64(task_ptr.origin_id), 10), "", task_ptr.resource_link, begin, end, task_ptr.margin,
								task_ptr.separate_entries, task_ptr.separate_properties, task_ptr.properties_amount,
								task_ptr.map_algorithm, task_ptr.map_algorithm_parameters, nil, nil,  nil, 0, false}
							task_ptr.jobs[strconv.FormatInt(int64(i), 10)] = jobs[i]
						}
					}
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
			//InfoLoggerPtr.Println("Difference", math.Abs(v2 - b_i[i2]), "margin", (v2 / 100) * float64(max_diff) )
			diff := math.Abs(v2 - b_i[i2])
			if diff > (math.Abs(v2) / 100) * float64(max_diff) && diff > (math.Abs(b_i[i2]) / 100) * float64(max_diff) {
				return false
			}

		}

	}

	/*
	for i, v_o := range a {
		v := v_o.(map[string]struct{})
		b_i := b[i].(map[string]struct{})
		if len(v) != len(b_i) {
			WarningLoggerPtr.Println("Lengths are supposed to be equal!")
			return false
		}

		for i2, v2 := range v {
			if v2 != b_i[i2] { return false }
		}

	}*/

	return true
}

func iteration_algorithm_clustering(task_ptr *task, new_task_ptr_ptr **task, keys_x_values map[string]interface{}) (bool) {
/*
	if task_ptr.iteration_algorithm_parameters == nil {
		task_ptr.iteration_algorithm_parameters = make([]interface{}, 0)
	}
*/
	if len(task_ptr.iteration_algorithm_parameters.([]interface{})) == 1 {
		task_ptr.iteration_algorithm_parameters = append(task_ptr.iteration_algorithm_parameters.([]interface{}), keys_x_values)
	} else {
		old_keys_x_values := task_ptr.iteration_algorithm_parameters.([]interface{})[1].(map[string]interface{})

		if iteration_algorithm_clustering_deep_equal(old_keys_x_values, keys_x_values, task_ptr.iteration_algorithm_parameters.([]interface{})[0].(int)) {
			InfoLoggerPtr.Println("Fixpoint found, iteration concluded")
			for key, key_value_o := range keys_x_values {
				key_value := key_value_o.([]float64)
				InfoLoggerPtr.Println("key", key, "value", key_value)

			}
			return true
		}

		task_ptr.iteration_algorithm_parameters.([]interface{})[1] = keys_x_values
	}

	for index_string, value := range keys_x_values {
		// i := 1
		// value := value_o.(map[string]struct{})
		index, _ := strconv.Atoi(index_string) // TODO check error
		task_ptr.map_algorithm_parameters.([]interface{})[index + 1] = value.([]float64)
		/*
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
			clustering_parameters[i] = point
		}
		*/
	}

	*new_task_ptr_ptr = &task{-1, task_ptr.origin_id, 0, task_ptr.resource_link, task_ptr.mappers_amount, task_ptr.margin,
		task_ptr.separate_entries, task_ptr.separate_properties, task_ptr.properties_amount, task_ptr.initialization_algorithm,
		task_ptr.map_algorithm, task_ptr.map_algorithm_parameters, task_ptr.reducers_amount, task_ptr.reduce_algorithm,
		task_ptr.reduce_algorithm_parameters, task_ptr.iteration_algorithm, task_ptr.iteration_algorithm_parameters, orderedmap.NewOrderedMap(), 0,
		make(map[string]*Job), make(map[string]*Job)}
	InfoLoggerPtr.Println("Fixpoint not found yet, new_task created")

	for index, index_value_o := range task_ptr.map_algorithm_parameters.([]interface{})[1:] {
		index_value := index_value_o.([]float64)
		InfoLoggerPtr.Println("key", index, "value", index_value)
	}
	/*
	for key, key_value_o := range keys_x_values {
		key_value := key_value_o.(map[string]struct{})
		for index, _ := range key_value {
			InfoLoggerPtr.Println("key", key, "value", index)
		}
	}*/
	return false
}

func iteration_manager(task_ptr *task, keys_x_values map[string]interface{}) {
	var new_task_ptr *task
	res, err := Call("iteration_algorithm_" + task_ptr.iteration_algorithm, stub_storage, task_ptr, &new_task_ptr, keys_x_values)
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
	go Rpc_job_goroutine(server_ptr, job_ptr, "Reducer_handler.Send_job",
			"Sent reducer job " + job_ptr.Id + " task " + job_ptr.Task_id,
			3, EXPIRE_TIME, false)

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
	servers_x_tasks_x_jobs_done := make(map[string]map[string]map[string]*Job) // Not needed...
	keys_x_values := make(map[string]interface{})

	for {
		select {
		case error_ptr := <-probable_reducer_error_channel:
			if error_ptr.keys_x_servers == nil && state == BUSY { // have to wait till mapper will produce the new keys_x_servers
				if el := task_hashmap.Front(); el != nil {
					if el.Key.(string) == error_ptr.task_id {
						WarningLoggerPtr.Println("Reducer state changing to WAIT because of mapper task", error_ptr.task_id)
						state = WAIT
					}
				}
			} else {
				if state == WAIT { state = BUSY }
				task_ptr_o, ok := task_hashmap.Get(error_ptr.task_id)
				if ok {
					task_ptr := task_ptr_o.(*task)
					task_ptr.keys_x_servers = error_ptr.keys_x_servers
					task_ptr.keys_x_servers_version += 1
					for _, server_ptr := range idle_reducer_hashmap {
						loop_break := false
						select {
						case job_ptr := <-job_channel:
							if job_ptr.Keys_x_servers_version != task_ptr.keys_x_servers_version {
								for key, key_content := range job_ptr.Keys_x_servers {
									servers_map, present := task_ptr.keys_x_servers.Get(key)
									if !present { ErrorLoggerPtr.Fatal("Key not found.") }
									key_content = servers_map.(map[string]*Server)

									// BUG compiler thinks i'm not using this variable... ?!?
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
					case keys_x_servers_update_channel <- error_ptr:
					default:
						ErrorLoggerPtr.Println("keys_x_servers_update_channel is full.")
					}
				}
			}
		case rem_reducer_ptr := <-rem_reducer_channel:
			delete(idle_reducer_hashmap, rem_reducer_ptr.Id)

			/*if _, ok := idle_reducer_hashmap[rem_reducer_ptr.Id]; ok {
				delete(idle_reducer_hashmap, rem_reducer_ptr.Id)
			} else if server, ok := working_reducer_hashmap[rem_reducer_ptr.Id]; ok {*/
			for el := task_hashmap.Back(); el != nil; el = el.Prev() {
				current_task_ptr := strconv.FormatInt(int64(el.Value.(*task).id), 10)
				jobs := servers_x_tasks_x_jobs[rem_reducer_ptr.Id][current_task_ptr]
				for _, job_ptr := range jobs {
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
							ErrorLoggerPtr.Fatal("job_channel queue full") // TODO handle this case...
						}
					}
				}
			}
			delete(working_reducer_hashmap, rem_reducer_ptr.Id)
			delete(servers_x_tasks_x_jobs, rem_reducer_ptr.Id)
			delete(servers_x_tasks_x_jobs_done, rem_reducer_ptr.Id)

		case add_reducer_ptr := <-add_reducer_channel:
			InfoLoggerPtr.Println("Reducer", add_reducer_ptr.Id, "ip", add_reducer_ptr.Ip, "port", add_reducer_ptr.Port, "is being added")
			servers_x_tasks_x_jobs[add_reducer_ptr.Id] = make(map[string]map[string]*Job)
			servers_x_tasks_x_jobs_done[add_reducer_ptr.Id] = make(map[string]map[string]*Job)

			if state != WAIT {
				select {
				case job_ptr := <-job_channel:
					assign_job_reducer(add_reducer_ptr, job_ptr, working_reducer_hashmap, servers_x_tasks_x_jobs)
				default:
					idle_reducer_hashmap[add_reducer_ptr.Id] = add_reducer_ptr
				}
			} else {
				idle_reducer_hashmap[add_reducer_ptr.Id] = add_reducer_ptr
			}
		case job_completed_ptr := <-Job_reducer_completed_channel:

			task_ptr_o, present := task_hashmap.Get(job_completed_ptr.Task_id)
			if !present { break }
			task_ptr := task_ptr_o.(*task)
			{
				job_map, ok := servers_x_tasks_x_jobs_done[job_completed_ptr.Server_id][job_completed_ptr.Task_id]
				if !ok {
					job_map = make(map[string]*Job)
					servers_x_tasks_x_jobs_done[job_completed_ptr.Server_id][job_completed_ptr.Task_id] = job_map
				}
				job_map[job_completed_ptr.Id] = job_completed_ptr
			}

			for key, key_value := range job_completed_ptr.Result {
				_, ok := keys_x_values[key]
				if !ok {
					keys_x_values[key] = key_value
				} else { // TODO hide unification algorithm... add it to the task as algorithm field...
					for hidden_index, hidden_value := range key_value.([]float64) {
						keys_x_values[key].([]float64)[hidden_index] = hidden_value
					}
				}
			}

			delete(task_ptr.jobs, job_completed_ptr.Id)
			task_ptr.jobs_done[job_completed_ptr.Id] = job_completed_ptr
			delete(servers_x_tasks_x_jobs[job_completed_ptr.Server_id][job_completed_ptr.Task_id], job_completed_ptr.Id)
			if len(servers_x_tasks_x_jobs[job_completed_ptr.Server_id][job_completed_ptr.Task_id]) == 0 {
				delete(servers_x_tasks_x_jobs[job_completed_ptr.Server_id], job_completed_ptr.Task_id)
			}

			if len(servers_x_tasks_x_jobs[job_completed_ptr.Server_id]) == 0 {
				if state != WAIT {
					select {
					case job_ptr := <-job_channel:
						assign_job_reducer(working_reducer_hashmap[job_completed_ptr.Server_id], job_ptr, working_reducer_hashmap, servers_x_tasks_x_jobs)
					default:
						idle_reducer_hashmap[job_completed_ptr.Server_id] = working_reducer_hashmap[job_completed_ptr.Server_id]
						delete(working_reducer_hashmap, job_completed_ptr.Server_id)
					}
				} else {
					idle_reducer_hashmap[job_completed_ptr.Server_id] = working_reducer_hashmap[job_completed_ptr.Server_id]
					delete(working_reducer_hashmap, job_completed_ptr.Server_id)
				}
				if len(working_reducer_hashmap) == 0 {
					InfoLoggerPtr.Println("Reducer job", job_completed_ptr.Id, "task", job_completed_ptr.Task_id, "completed.")
					state = IDLE
					select {
					case task_reducer_completed_channel <-job_completed_ptr.Task_id:
					default:
						ErrorLoggerPtr.Fatal("Task_reducer_completed full.")
					}
					task_ptr, _ := task_hashmap.Get(job_completed_ptr.Task_id) // TODO check error
					go iteration_manager(task_ptr.(*task), keys_x_values)
					keys_x_values = make(map[string]interface{})
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
			if len(working_reducer_hashmap) == 0 { // if the curent task finished
				select {
				case task_ptr := <-Task_reducer_channel:
					for loop := true; loop; {
						select {
						case error_ptr := <-keys_x_servers_update_channel:
							if strconv.FormatInt(int64(task_ptr.id), 10) == error_ptr.task_id {
								task_ptr.keys_x_servers = error_ptr.keys_x_servers
								task_ptr.keys_x_servers_version += 1
							}
						default:
							loop = false
						}
					}
					task_hashmap.Set(strconv.Itoa(int(task_ptr.id)), task_ptr) // TODO check overflow
					state = BUSY
					InfoLoggerPtr.Println("Scheduling reducer task:", task_ptr.id)
					keys_amount := int32(task_ptr.keys_x_servers.Len()) // TODO check overflow
					reducers_amount := MinOf_int32(task_ptr.reducers_amount, int32(len(idle_reducer_hashmap))) // TODO check overflow
					slice_size := 1
					if reducers_amount == 0 { reducers_amount = 1 }
					var slice_rest int32 = 0
					if keys_amount > reducers_amount {
						slice_size = int(math.Abs(float64(keys_amount) / float64(reducers_amount))) // TODO check overflow
						slice_rest = keys_amount % reducers_amount
					}
					if reducers_amount == 0 { reducers_amount = 1 }

					jobs := make([]*Job, reducers_amount)
					{
						var i int32 = 0
						for ; i < reducers_amount; i += 1 {
							current_slice_size := slice_size
							if slice_rest > 0 {
								current_slice_size += 1
								slice_rest -= 1
							}

							// TODO copy it in map side too ! Possible BUG segmentation fault !
							keys := make(map[string]map[string]*Server)
							{
								j := 0
								el := task_ptr.keys_x_servers.Front()
								//InfoLoggerPtr.Println("size", current_slice_size)
								for ; j < current_slice_size || el != nil; j += 1 {
									value := el.Value.(map[string]*Server)
									keys[el.Key.(string)] = value
									el = el.Next()
								}
								if j < current_slice_size && el == nil {
									ErrorLoggerPtr.Fatal("There are less keys than expected.")
								}
							}

							jobs[i] = &Job{strconv.FormatInt(int64(i), 10), strconv.FormatInt(int64(task_ptr.id), 10),
								strconv.FormatInt(int64(task_ptr.id), 10), "", "", 0, 0, 0, task_ptr.separate_entries,
								task_ptr.separate_properties, task_ptr.properties_amount, task_ptr.reduce_algorithm,
								task_ptr.reduce_algorithm_parameters, nil, nil, keys, 0, false}
							task_ptr.jobs[strconv.FormatInt(int64(i), 10)] = jobs[i]
						}
					}
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


func Master_main() {

	rand.Seed(time.Now().UnixNano())

	gob.Register([]interface{}(nil))

	stub_storage = map[string]interface{}{
		"iteration_algorithm_clustering": iteration_algorithm_clustering,
		"initialization_algorithm_clustering": initialization_algorithm_clustering,
		//"funcB": funcB,
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
	Task_mapper_channel = make(chan *task, 1000)

	//creating channel for communicating new task
	Task_reducer_channel = make(chan *task, 1000)

	Job_reducer_completed_channel = make(chan *Job, 1000)

	task_reducer_completed_channel = make(chan string, 1000)

	probable_reducer_error_channel = make(chan *map_to_reduce_error, 1000)

	go scheduler_reducer_goroutine()
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

