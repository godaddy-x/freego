package main

import (
	"fmt"
	"github.com/godaddy-x/freego/job"
	"github.com/godaddy-x/freego/util"
	"testing"
)

func TestJobTask(t *testing.T) {
	task1 := job.Task{
		Spec: "*/5 * * * * *",
		Func: func() {
			fmt.Println("job task1: ", util.Time())
		},
	}
	task2 := job.Task{
		Spec: "*/10 * * * * *",
		Func: func() {
			fmt.Println("job task2: ", util.Time())
		},
	}
	job.Run(task1, task2)
}
