package main

import (
//	"fmt"
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

const APP_ID = "SDCC"

const MASTER_IP = "172.18.0.254"

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


func main() {

	InfoLoggerPtr.Println("Starting app...")

	for true {
		switch os.Getenv("SDCC_ROLE") {
		case MASTER:
			InfoLoggerPtr.Println("Starting master...")
			master_main()
			goto exit
		case MAPPER:
			InfoLoggerPtr.Println("Starting mapper...")
			mapper_main()
			goto exit
		case REDUCER:
			InfoLoggerPtr.Println("Starting reducer...")
			goto exit
		default:
			WarningLoggerPtr.Println("SDCC_ROLE env is not set correctly.")
			time.Sleep(time.Second)
		}
	}
exit:
os.Exit(EXIT_SUCCESS)
}

