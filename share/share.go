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

func init() {

	InfoLoggerPtr = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	WarningLoggerPtr = log.New(os.Stdout, "WARNING: ", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLoggerPtr = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

}


