package main

import (
	"fmt"
	"github.com/godaddy-x/freego/job"
	"github.com/godaddy-x/freego/utils"
	"testing"
)

func TestJobTask(t *testing.T) {
	task1 := job.Task{
		Spec: "*/5 * * * * *",
		Func: func() {
			fmt.Println("job task1: ", utils.UnixMilli())
		},
	}
	task2 := job.Task{
		Spec: "*/10 * * * * *",
		Func: func() {
			fmt.Println("job task2: ", utils.UnixMilli())
		},
	}
	job.Run(task1, task2)
}
