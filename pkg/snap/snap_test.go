package snap

import (
	"testing"
	"fmt"
)

func TestCreateTask(t *testing.T) {

	manager, _ := NewTaskManager("localhost")
	task := NewTask("test")
	fmt.Print("task name: " + task.Name)


	id, _ := manager.CreateTask(task)


	fmt.Printf("id: ", id)

}

func TestDelTask(t *testing.T){
	manager, _ := NewTaskManager("localhost")

	manager.StopTask("2a405aa5-809c-4a6a-87b1-529b55b214ea")
	manager.RemoveTask("2a405aa5-809c-4a6a-87b1-529b55b214ea")
}
