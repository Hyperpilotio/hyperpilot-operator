package snap

import (
	"fmt"
	"testing"
)

func TestCreateTask(t *testing.T) {

	manager, _ := NewTaskManager("35.197.81.238")
	//manager, _ := NewTaskManager("localhost")
	task := NewTask("OGRE")
	fmt.Print("task name: " + task.Name)

	id, err := manager.CreateTask(task)
	if err != nil {
		fmt.Printf(err.Error())
	}
	fmt.Printf("id: ", id)

}

func TestDelTask(t *testing.T) {
	manager, _ := NewTaskManager("35.197.81.238")

	manager.StopTask("f2ac16b5-94e3-405e-8cdc-7d7aa0324c6c")
	manager.RemoveTask("f2ac16b5-94e3-405e-8cdc-7d7aa0324c6c")
}
