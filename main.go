package main

import (
	"fmt"
	"os"
	"time"
)

// For simplicity we just suppose this is true
const (
	EXIT_SUCCESS = 0
	EXIT_FAILURE = 1
)

const (
	MASTER	= "MASTER"
	MAPPER	= "MAPPER"
	REDUCER	= "REDUCER"
)


func main() {

	for true {
		switch os.Getenv("SDCC_ROLE") {
		case MASTER:
			fmt.Println(MASTER)
			goto exit
		case MAPPER:
			fmt.Println(MAPPER)
			goto exit
		case REDUCER:
			fmt.Println(REDUCER)
			goto exit
		default:
			time.Sleep(time.Second)
		}
	}
exit:
os.Exit(EXIT_SUCCESS)
}

