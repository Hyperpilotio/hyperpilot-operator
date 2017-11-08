package snap

import (
	"testing"
)

func TestCreateTask(t *testing.T) {

	manager, _ := NewTaskManager("localhost")

	tasl := NewPrometheusCollectorTask("172.17", "0.3", 7998)

	manager.CreateTask(tasl)

}
