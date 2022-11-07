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


func Rpc_request_goroutine(server *Server, load *Request, method string,  log_message string) (interface{}) {

	// TODO probably it is needed to use the already connection which is in place for the heartbeat

	// connect to server via rpc tcp
	client, err := rpc.Dial("tcp", server.Ip + ":" + server.Port)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	var reply interface{}

	err = client.Call(method, load, &reply)
	if err != nil {
		ErrorLoggerPtr.Fatal(method, "error", err)
	}
	InfoLoggerPtr.Println(log_message)
	return reply
}


func Rpc_job_goroutine(server_ptr *Server, job_ptr *Job, method string, log_message string) {
	// connect to mapper via rpc tcp
	client, err := rpc.Dial("tcp", server_ptr.Ip + ":" + server_ptr.Port)
	defer client.Close()
	if err != nil {
		ErrorLoggerPtr.Fatal(err)
	}

	var reply int

	err = client.Call(method, job_ptr, &reply)
	if err != nil {
		ErrorLoggerPtr.Fatal(method, "error:", err)
	}
	InfoLoggerPtr.Println(log_message)

}

