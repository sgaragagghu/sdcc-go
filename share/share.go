package share

import (
	"os"
	"time"
	"log"
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
)

const APP_ID = "SDCC"

const MASTER_IP = "172.18.0.254"

const SECOND = time.Second

var (
	WarningLoggerPtr	*log.Logger
	InfoLoggerPtr		*log.Logger
	ErrorLoggerPtr		*log.Logger
)

type Server struct {
	Id string
	Ip string
	Port string
	Last_heartbeat time.Time
	Jobs map[string]*Job
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
	Map_algorithm string
	Map_algorithm_parameters interface{}
	Map_result map[string]interface{}
	Map_keys []string
	Reducers_amount int32
	Reduce_algorithm string
	Reduce_algorithm_parameters interface{}
	Reduce_result map[string]interface{}
	Reduce_keys []string
	Iteration_algorithm string
	Iteration_algorithm_parameters interface{}
	Keys_x_servers map[string]map[string]*Server
	Delete bool
}

func init() {

	InfoLoggerPtr = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	WarningLoggerPtr = log.New(os.Stdout, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLoggerPtr = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

}


