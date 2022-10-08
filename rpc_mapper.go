package main

import (
//	"time"
)

const MAPPER_PORT = "6668"

var (
	Task_channel_ptr *chan *Task
)

// create a type to get an interface
type Mapper_handler int

type Task struct {
	Id string
	Payload string
}


func (h Mapper_handler) Send_task(args *Task, reply *int) error {
	*Task_channel_ptr <- args
	return nil
}

