package snap

import (
	"errors"
	"github.com/intelsdi-x/snap/mgmt/rest/client"
	"github.com/intelsdi-x/snap/scheduler/wmap"
)

const (
	SNAP_VERSION                = "v1"
	PROMETHEUS_TASK_NAME_PREFIX = "PROMETHEUS"
)

type RunOpts struct {
	ScheduleType      string
	ScheduleInterval  string
	StartTaskOnCreate bool
	MaxFailure        int
}

type TaskManager struct {
	*client.Client
}

type Task struct {
	Name        string
	WorkflowMap *wmap.WorkflowMap
	Opts        *RunOpts
}

func NewTaskManager(podIP string) (*TaskManager, error) {

	url := "http://" + podIP + ":8181"
	snapClient, err := client.New(url, SNAP_VERSION, true)

	if err != nil {
		return nil, err
	}
	return &TaskManager{
		Client: snapClient,
	}, nil
}

func NewTask(podName string) *Task {

	runOpts := &RunOpts{
		ScheduleType:      "simple",
		ScheduleInterval:  "5s",
		StartTaskOnCreate: true,
		MaxFailure:        -1,
	}

	return &Task{
		// e.g. PROMETHEUS-resource-worker-spark-9br5d
		Name:        PROMETHEUS_TASK_NAME_PREFIX + "-" + podName,
		WorkflowMap: NewWorkflowMap(podName),
		Opts:        runOpts,
	}
}

// todo: define actual workflow
// test workflow, install following plugin first
// 	"snap-plugin-processor-tag_linux_x86_64",
// 	"snap-plugin-publisher-file_linux_x86_64",
// 	"snap-plugin-collector-cpu_linux_x86_64",
func NewWorkflowMap(name string) *wmap.WorkflowMap {

	ns := "/intel/procfs"
	metrics := make(map[string]int)
	metrics["/intel/procfs/cpu/*/user_jiffies"] = 7
	config := make(map[string]interface{})
	config["proc_path"] = "/proc"
	collector := NewCollector(ns, metrics, config)

	// create processor
	processorConfig := make(map[string]interface{})
	processorConfig["tags"] = "tag1:value1,tag2:value2"
	processor := NewProcessor("tag", 3, processorConfig)

	// create publisher
	publisherConfig := make(map[string]interface{})
	publisherConfig["file"] = "/tmp/task.log"
	publisher := NewPublisher("file", 2, publisherConfig)

	return publisher.JoinProcessor(processor).Join(collector).Join(wmap.NewWorkflowMap())
}

func (manager *TaskManager) CreateTask(task *Task) (taskId string, err error) {
	sch := &client.Schedule{Type: task.Opts.ScheduleType, Interval: task.Opts.ScheduleInterval}
	result := manager.Client.CreateTask(sch, task.WorkflowMap, task.Name, "", task.Opts.StartTaskOnCreate, task.Opts.MaxFailure)

	if result.Err == nil {
		return result.ID, nil
	}
	return "", errors.New("Create Task Failed.")
}
