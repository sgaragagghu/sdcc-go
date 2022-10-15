package main

import (
	. "./master"
	. "./mapper"
	. "./share"
	"os"
	"time"
	"log"
)

func main() {

	InfoLoggerPtr.Println("Starting app...")

	for true {
		switch os.Getenv("SDCC_ROLE") {
		case MASTER:
			InfoLoggerPtr.Println("Starting master...")
			Master_main()
			goto exit
		case MAPPER:
			InfoLoggerPtr.Println("Starting mapper...")
			Mapper_main()
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

