package share

import (
	"os"
	"time"
	"log"
	"net/rpc"
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
	Id string
	Task_id string
	Origin_task_id string
	Server_id string
	Resource_link string
	Begin int64
	End int64
	Margin int8
	Separate_entries byte
	Separate_properties byte
	Properties_amount int8
	Algorithm string
	Algorithm_parameters interface{}
	Result map[string]interface{}
	Keys []string
	Keys_x_servers map[string]map[string]*Server
	Keys_x_servers_version int8
	Delete bool
}

func init() {

	InfoLoggerPtr = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	WarningLoggerPtr = log.New(os.Stdout, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLoggerPtr = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

}

func Rpc_request_goroutine(server *Server, load *Request, method string,  log_message string, retry int, delay time.Duration, error_is_fatal bool, call_bug bool) (interface{}) {

	// TODO probably it is needed to use the already connection which is in place for the heartbeat

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
		if call_bug {
			err = nil
			break
		}

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


func Rpc_job_goroutine(server_ptr *Server, job_ptr *Job, method string, log_message string, retry int, delay time.Duration, error_is_fatal bool) {
	// connect to mapper via rpc tcp
	client, err := rpc.Dial("tcp", server_ptr.Ip + ":" + server_ptr.Port)
	defer client.Close()
	if err != nil {
		if error_is_fatal {
			ErrorLoggerPtr.Fatal(err)
		} else {
			ErrorLoggerPtr.Println(err)
		}
	}

	var reply int

	err = client.Call(method, job_ptr, &reply)
	if err != nil {
		if error_is_fatal {
			ErrorLoggerPtr.Fatal(method, " error ", err)
		} else {
			ErrorLoggerPtr.Println(method, " error ", err)
		}
	}
	InfoLoggerPtr.Println(log_message)
}

