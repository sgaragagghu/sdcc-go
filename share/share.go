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
	Algorithm string
	Algorithm_parameters interface{}
	Result map[string]interface{}
	Keys []string
	Keys_x_servers map[string]map[string]*Server
	Delete bool
}

func init() {

	InfoLoggerPtr = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	WarningLoggerPtr = log.New(os.Stdout, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLoggerPtr = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

}


