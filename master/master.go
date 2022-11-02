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

type task struct {
	id int32
	origin_id int32
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

	stub_storage StubMapping
)

func task_injector_goroutine() { // TODO make a jsonrpc interface to send tasks from a browser or curl 

	time.Sleep(60 * SECOND)

	parameters := make([]interface{}, 5)
	parameters[0] = 4 // k
	parameters[1] = []float64{0, 0} // u_0
	parameters[2] = []float64{1, 1} // u_1
	parameters[3] = []float64{2, 2} // u_2
	parameters[4] = []float64{3, 3} // u_3


	iteration_parameters := make([]interface{}, 1)
	iteration_parameters[0] =  1 // max_diff (percentage)


/*
	parameters_ptr := new(list.List)
	parameters_ptr.PushFront(4) // k
	parameters_ptr.PushFront([]int{0, 0}) // u_0
	parameters_ptr.PushFront([]int{1, 1}) // u_1
	parameters_ptr.PushFront([]int{2, 2}) // u_2
	parameters_ptr.PushFront([]int{3, 3}) // u_3
*/
	task_ptr := &task{-1, -1, "https://raw.githubusercontent.com/sgaragagghu/sdcc-clustering-datasets/master/sdcc/2d-4c.csv", 1, 10,
		'\n', ',', 2, "clustering", "clustering", parameters, 1, "clustering", nil, "clustering", iteration_parameters, nil}


	// TODO MOVE TO initialization_algorithm_clustering FUNCTION (TO DO!)
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


		reader := bytes.NewReader((*load_ptr)[actual_begin:actual_end])
		buffered_read := bufio.NewReader(reader)
		Parser_simple(&(offsets[i]), buffered_read, task_ptr.separate_properties, task_ptr.separate_entries)

	}

	for i, v := range offsets {
		task_ptr.map_algorithm_parameters.([]interface{})[i + 1] = v
	}

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
	task_hashmap := make(map[string]*task)
	keys_x_servers := orderedmap.NewOrderedMap()

	for {
		select {
		case rem_mapper_ptr := <-rem_mapper_channel:
			if _, ok := idle_mapper_hashmap[rem_mapper_ptr.Id]; ok {
				delete(idle_mapper_hashmap, rem_mapper_ptr.Id)
			} else if server, ok := working_mapper_hashmap[rem_mapper_ptr.Id]; ok {
				jobs := server.Jobs
				for _, job_ptr := range jobs {
					if len(idle_mapper_hashmap) > 0 {
						for server_id, server_ptr := range idle_mapper_hashmap {
							delete(idle_mapper_hashmap, server_id)
							job_ptr.Server_id = server_id
							working_mapper_hashmap[server_id] = server_ptr
							InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to mapper", job_ptr.Server_id)
							go Rpc_job_goroutine(server_ptr, job_ptr, "Mapper_handler.Send_job",
								"Sent mapper job " + job_ptr.Id + " task " + job_ptr.Task_id)
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
			InfoLoggerPtr.Println("Mapper", add_mapper_ptr.Id, "ip", add_mapper_ptr.Ip, "port", add_mapper_ptr.Port, "is being added")
			mapper_job_map := make(map[string]*Job)
			add_mapper_ptr.Jobs = mapper_job_map
			select {
			case job_ptr := <-job_channel:
				job_ptr.Server_id = add_mapper_ptr.Id
				working_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
				add_mapper_ptr.Jobs[job_ptr.Id] = job_ptr
				InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to mapper", job_ptr.Server_id)
				go Rpc_job_goroutine(add_mapper_ptr, job_ptr, "Mapper_handler.Send_job",
					"Sent mapper job " + job_ptr.Id + " task " + job_ptr.Task_id)
			default:
				idle_mapper_hashmap[add_mapper_ptr.Id] = add_mapper_ptr
			}
		case job_completed_ptr := <-Job_mapper_completed_channel:
			mapper_job_map_ptr := working_mapper_hashmap[job_completed_ptr.Server_id].Jobs
			delete(mapper_job_map_ptr, job_completed_ptr.Id)

			for _, v := range job_completed_ptr.Keys {
				value, ok := keys_x_servers.Get(v)
				if !ok {
					value = make(map[string]*Server)
					keys_x_servers.Set(v, value)
				}
				server := working_mapper_hashmap[job_completed_ptr.Server_id]
				// TODO manage it in a way to not waste memory
				server_light := Server{server.Id, server.Ip, server.Port, server.Last_heartbeat, nil, server.Role}
				value.(map[string]*Server)[job_completed_ptr.Server_id] = &server_light
			}

			if len(mapper_job_map_ptr) == 0 {
				select {
				case job_ptr := <-job_channel:
					job_ptr.Server_id = job_completed_ptr.Server_id
					InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to mapper", job_ptr.Server_id)
					mapper_job_map_ptr[job_ptr.Id] = job_ptr
					go Rpc_job_goroutine(working_mapper_hashmap[job_completed_ptr.Server_id], job_ptr, "Mapper_handler.Send_job",
						"Sent mapper job " + job_ptr.Id + " task " + job_ptr.Task_id)
				default:
					idle_mapper_hashmap[job_completed_ptr.Server_id] = working_mapper_hashmap[job_completed_ptr.Server_id]
					delete(working_mapper_hashmap, job_completed_ptr.Server_id)
				}
				if len(working_mapper_hashmap) == 0 {
					InfoLoggerPtr.Println("Mapper job", job_completed_ptr.Id, "task", job_completed_ptr.Task_id, "completed.")
					state = IDLE

					task_ptr, _ := task_hashmap[job_completed_ptr.Task_id]
					task_ptr.keys_x_servers = keys_x_servers

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

				}
				if state == IDLE && len(Task_mapper_channel) > 0 { // if the curent task finished and theres a task
					select {
					case New_task_mapper_event_channel <-struct{}{}:
					default:
						ErrorLoggerPtr.Fatal("New_task_mapper_event_channel full.")
					}
					keys_x_servers = orderedmap.NewOrderedMap()
				}
			}
		case <-New_task_mapper_event_channel:
			if len(working_mapper_hashmap) == 0 { // if the curent task finished
				select {
				case task_ptr := <-Task_mapper_channel:
					task_ptr.id = task_counter
					task_counter += 1
					state = BUSY
					task_hashmap[strconv.Itoa(int(task_ptr.id))] = task_ptr // TODO check overflow
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
								task_ptr.map_algorithm, task_ptr.map_algorithm_parameters, nil, nil,  nil, false}
						}
					}
					if len(idle_mapper_hashmap) > 0 {
						var i int32 = 0
						for _, server_ptr := range idle_mapper_hashmap {
							if i >= mappers_amount { break }
							jobs[i].Server_id = server_ptr.Id
							working_mapper_hashmap[server_ptr.Id] = server_ptr
							server_ptr.Jobs[jobs[i].Id] = jobs[i]
							go Rpc_job_goroutine(server_ptr, jobs[i], "Mapper_handler.Send_job",
								"Sent mapper job " + jobs[i].Id + " task " + jobs[i].Task_id)
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
			if math.Abs(v2 - b_i[i2]) > (v2 / 100) * float64(max_diff) { return false }
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

	*new_task_ptr_ptr = &task{-1, task_ptr.origin_id, task_ptr.resource_link, task_ptr.mappers_amount, task_ptr.margin,
		task_ptr.separate_entries, task_ptr.separate_properties, task_ptr.properties_amount, task_ptr.initialization_algorithm,
		task_ptr.map_algorithm, task_ptr.map_algorithm_parameters, task_ptr.reducers_amount, task_ptr.reduce_algorithm,
		task_ptr.reduce_algorithm_parameters, task_ptr.iteration_algorithm, task_ptr.iteration_algorithm_parameters, nil}
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

func scheduler_reducer_goroutine() {
	InfoLoggerPtr.Println("Scheduler_reducer_goroutine started.")

	state := IDLE
	job_channel := make(chan *Job, 1000)
	idle_reducer_hashmap := make(map[string]*Server)
	working_reducer_hashmap := make(map[string]*Server)
	task_hashmap := make(map[string]*task)
	keys_x_values := make(map[string]interface{})

	for {
		select {
		case rem_reducer_ptr := <-rem_reducer_channel:
			if _, ok := idle_reducer_hashmap[rem_reducer_ptr.Id]; ok {
				delete(idle_reducer_hashmap, rem_reducer_ptr.Id)
			} else if server, ok := working_reducer_hashmap[rem_reducer_ptr.Id]; ok {
				jobs := server.Jobs
				for _, job_ptr := range jobs {
					if len(idle_reducer_hashmap) > 0 {
						for server_id, server_ptr := range idle_reducer_hashmap {
							delete(idle_reducer_hashmap, server_id)
							job_ptr.Server_id = server_id
							working_reducer_hashmap[server_id] = server_ptr
							InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to reducer", job_ptr.Server_id)
							go Rpc_job_goroutine(server_ptr, job_ptr, "Reducer_handler.Send_job",
								"Sent reducer job " + job_ptr.Id + " task " + job_ptr.Task_id)
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
			delete(working_reducer_hashmap, rem_reducer_ptr.Id)
			}
		case add_reducer_ptr := <-add_reducer_channel:
			InfoLoggerPtr.Println("Reducer", add_reducer_ptr.Id, "ip", add_reducer_ptr.Ip, "port", add_reducer_ptr.Port, "is being added")
			reducer_job_map := make(map[string]*Job)
			add_reducer_ptr.Jobs = reducer_job_map
			select {
			case job_ptr := <-job_channel:
				job_ptr.Server_id = add_reducer_ptr.Id
				working_reducer_hashmap[add_reducer_ptr.Id] = add_reducer_ptr
				add_reducer_ptr.Jobs[job_ptr.Id] = job_ptr
				InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to reducer", job_ptr.Server_id)
				go Rpc_job_goroutine(add_reducer_ptr, job_ptr, "Reducer_handler.Send_job",
					"Sent reducer job " + job_ptr.Id + " task " + job_ptr.Task_id)
			default:
				idle_reducer_hashmap[add_reducer_ptr.Id] = add_reducer_ptr
			}
		case job_completed_ptr := <-Job_reducer_completed_channel:

			for key, key_value := range job_completed_ptr.Result {
				_, ok := keys_x_values[key]
				if !ok {
					keys_x_values[key] = key_value
				} else { // TODO hide unification algorithm... add it to the task as algorithm field...
					for hidden_index, hidden_value := range key_value.(map[string]struct{}) {
						keys_x_values[key].(map[string]struct{})[hidden_index] = hidden_value
					}
				}
			}

			reducer_job_map_ptr := working_reducer_hashmap[job_completed_ptr.Server_id].Jobs
			delete(reducer_job_map_ptr, job_completed_ptr.Id)
			if len(reducer_job_map_ptr) == 0 {
				select {
				case job_ptr := <-job_channel:
					job_ptr.Server_id = job_completed_ptr.Server_id
					InfoLoggerPtr.Println("Job", job_ptr.Id, "assigned to reducer", job_ptr.Server_id)
					reducer_job_map_ptr[job_ptr.Id] = job_ptr
					go Rpc_job_goroutine(working_reducer_hashmap[job_completed_ptr.Server_id], job_ptr, "Reducer_handler.Send_job",
						"Sent reducer job " + job_ptr.Id + " task " + job_ptr.Task_id)
				default:
					idle_reducer_hashmap[job_completed_ptr.Server_id] = working_reducer_hashmap[job_completed_ptr.Server_id]
					delete(working_reducer_hashmap, job_completed_ptr.Server_id)
				}
				if len(working_reducer_hashmap) == 0 {
					InfoLoggerPtr.Println("Reducer job", job_completed_ptr.Id, "task", job_completed_ptr.Task_id, "completed.")
					state = IDLE
					task_ptr := task_hashmap[job_completed_ptr.Task_id]
					go iteration_manager(task_ptr, keys_x_values)
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
					task_hashmap[strconv.Itoa(int(task_ptr.id))] = task_ptr // TODO check overflow
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
								task_ptr.reduce_algorithm_parameters, nil, nil, keys, false}
						}
					}
					if len(idle_reducer_hashmap) > 0 {
						i := 0
						for _, server_ptr := range idle_reducer_hashmap {
							jobs[i].Server_id = server_ptr.Id
							working_reducer_hashmap[server_ptr.Id] = server_ptr
							server_ptr.Jobs[jobs[i].Id] = jobs[i]
							go Rpc_job_goroutine(server_ptr, jobs[i], "Reducer_handler.Send_job",
								"Sent reducer job " + jobs[i].Id + " task " + jobs[i].Task_id)
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

	stub_storage = map[string]interface{}{
		"iteration_algorithm_clustering": iteration_algorithm_clustering,
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

