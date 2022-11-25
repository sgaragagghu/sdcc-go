package share

import (
	"os"
	"time"
	"log"
	"net/rpc"
	"reflect"
)

// tech debt
const (
	EXIT_SUCCESS = 0
	EXIT_FAILURE = 1
)

const (
	MASTER	= "MASTER"
	MAPPER	= "MAPPER"
	REDUCER	= "REDUCER"
)

const (
	IDLE = "IDLE"
	BUSY = "BUSY"
	WAIT = "WAIT"
)

const EXPIRE_TIME = 10

const APP_ID = "SDCC"

const MASTER_IP = "172.18.0.254"

const MAX_MAP_TASKS = 2

const SECOND = time.Second

var (
	WarningLoggerPtr	*log.Logger
	InfoLoggerPtr		*log.Logger
	ErrorLoggerPtr		*log.Logger
)

type Request struct {
	Sender *Server
	Receiver *Server
	Tries int
	Time time.Time
	Body interface{}
}

type Server struct {
	Id string
	Ip string
	Port string
	Last_heartbeat time.Time
	Role string
}

type Job struct {
	Id string // job id
	Task_id string // task id
	Origin_task_id string // task id of the first iteration
	Server_id string // server id
	Resource_link string // it is supposed that we are using an http resource link 
	Begin int64 // start of the assigned slice
	End int64 // end of the assigned slice
	Margin int8 // mapper doesn't download exactly its assigned slice but a bit more, it is explained in the mapper source code
	Separate_entries byte // (parser) character placed between entries
	Separate_properties byte // (parser) character placed between properties
	Properties_amount int8 // amount of properties for each entries (eg. each cartesian plane's point has 2 properties)
	Algorithm map[string]string // various algorithm that can be helpful to the job
	Algorithm_parameters map[string]interface{} // same but for prameters
	Result map[string]interface{}
	Keys []string
	Keys_x_servers map[string]map[string]*Server // amount of properties for each entries (eg. each cartesian plane's point has 2 properties)
	Keys_x_servers_version int8 // if a mapper crashes then probably we have to modify the previous variable and then increment this
	Delete bool
}

func init() {

	InfoLoggerPtr = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	WarningLoggerPtr = log.New(os.Stdout, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLoggerPtr = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

}
// custom function for combining keys values
func Join_algorithm_clustering (a interface{}, b interface{}, _ interface{}) (ret struct{}) {
	// This version of Go do not implement generics.
	map_string_struct := reflect.TypeOf(make(map[string]struct{}))
	slice_float64 := reflect.TypeOf(make([]float64, 0))

	if (reflect.TypeOf(a) != reflect.TypeOf(b)) { ErrorLoggerPtr.Fatal("Join falied: different kind!") }

	if map_string_struct == reflect.TypeOf(a) {
		for hidden_index, hidden_value := range b.(map[string]struct{}) {
			a.(map[string]struct{})[hidden_index] = hidden_value
		}
	} else if slice_float64 == reflect.TypeOf(a) {
		for hidden_index, hidden_value := range b.([]float64) {
			a.([]float64)[hidden_index] = hidden_value
		}
	} else {
		ErrorLoggerPtr.Println("Unexpected type!")
	}
	return
}
// connect to the rpc server and call the remote method "method" and paremeters "load" (type request)
func Rpc_request_goroutine(server *Server, load *Request, method string,  log_message string, retry int, delay time.Duration, error_is_fatal bool) (interface{}) {

	// connect to server via rpc tcp
	var client *rpc.Client
	var err error = nil
	for ; retry > 0; retry -= 1 {
		client, err = rpc.Dial("tcp", server.Ip + ":" + server.Port)
		if err == nil {
			break
		} else {
			WarningLoggerPtr.Println("rpc dial, server", server.Id, "connection try", retry, "failed", err)
			time.Sleep(delay * SECOND)
		}
	}
	defer client.Close()
	if err != nil {
		if error_is_fatal {
			ErrorLoggerPtr.Fatal(err)
		} else {
			ErrorLoggerPtr.Println(err)
		}
	}

	var reply interface{}

	for ; retry > 0; retry -= 1 {
		err = client.Call(method, load, &reply)

		if err == nil {
			break
		} else {
			WarningLoggerPtr.Println("rpc call, server", server.Id, "connection try", retry, "failed", err)
			time.Sleep(delay * SECOND)
		}
	}
	if err != nil {
		if error_is_fatal {
			ErrorLoggerPtr.Fatal(method, "error", err)
		} else {
			ErrorLoggerPtr.Println(method, "error", err)
		}
	}
	InfoLoggerPtr.Println(log_message)
	return reply
}

// connect to the rpc server and call the remote method "method" and paremeters "load" (type Job)
func Rpc_job_goroutine(server *Server, load *Job, method string, log_message string, retry int, delay time.Duration, error_is_fatal bool) (interface{}) {
	// connect to mapper via rpc tcp
	var client *rpc.Client
	var err error = nil
	for ; retry > 0; retry -= 1 {
		client, err = rpc.Dial("tcp", server.Ip + ":" + server.Port)
		if err == nil {
			break
		} else {
			WarningLoggerPtr.Println("rpc dial, server", server.Id, "connection try", retry, "failed", err)
			time.Sleep(delay * SECOND)
		}
	}
	defer client.Close()
	if err != nil {
		if error_is_fatal {
			ErrorLoggerPtr.Fatal(err)
		} else {
			ErrorLoggerPtr.Println(err)
		}
	}

	var reply interface{}

	for ; retry > 0; retry -= 1 {
		err = client.Call(method, load, &reply)

		if err == nil {
			break
		} else {
			WarningLoggerPtr.Println("rpc call, server", server.Id, "connection try", retry, "failed", err)
			time.Sleep(delay * SECOND)
		}
	}
	if err != nil {
		if error_is_fatal {
			ErrorLoggerPtr.Fatal(method, "error", err)
		} else {
			ErrorLoggerPtr.Println(method, "error", err)
		}
	}
	InfoLoggerPtr.Println(log_message)
	return reply
}

