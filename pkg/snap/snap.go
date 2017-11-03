package snap

import (
	"errors"
	"strconv"

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
		return nil, errors.New("Unable to create snap client: " + err.Error())
	}

	return &TaskManager{
		Client: snapClient,
	}, nil
}

func NewPrometheusCollectorTask(podName string, namespace string, port int32) *Task {
	runOpts := &RunOpts{
		ScheduleType:      "simple",
		ScheduleInterval:  "5s",
		StartTaskOnCreate: true,
		MaxFailure:        -1,
	}

	return &Task{
		// e.g. PROMETHEUS-resource-worker-spark-9br5d
		Name:        PROMETHEUS_TASK_NAME_PREFIX + "-" + podName,
		WorkflowMap: NewPrometheusCollectorWorkflowMap(podName, namespace, port),
		Opts:        runOpts,
	}
}

// todo: define actual workflow
// test workflow, install following plugin first
// 	"snap-plugin-processor-tag_linux_x86_64",
// 	"snap-plugin-publisher-file_linux_x86_64",
// 	"snap-plugin-collector-cpu_linux_x86_64",
func NewPrometheusCollectorWorkflowMap(podName string, namespace string, port int32) *wmap.WorkflowMap {
	ns := "/hyperpilot/prometheus"
	metrics := make(map[string]int)
	metrics["/hyperpilot/prometheus/*"] = 1
	config := make(map[string]interface{})
	config["endpoint"] = "http://" + podName + "." + namespace + ":" + strconv.Itoa(int(port))
	collector := NewCollector(ns, metrics, config)

	// create influx publisher
	publisherConfig := make(map[string]interface{})
	publisherConfig["host"] = "influxsrv.hyperpilot"
	publisherConfig["port"] = 8086
	publisherConfig["database"] = "snap"
	publisherConfig["user"] = "root"
	publisherConfig["password"] = "hyperpilot"
	publisherConfig["https"] = false
	publisherConfig["skip-verify"] = false
	publisher := NewPublisher("influxdb", 2, publisherConfig)

	return publisher.JoinCollector(collector).Join(wmap.NewWorkflowMap())
}

func (manager *TaskManager) CreateTask(task *Task) (string, error) {
	sch := &client.Schedule{Type: task.Opts.ScheduleType, Interval: task.Opts.ScheduleInterval}
	result := manager.Client.CreateTask(sch, task.WorkflowMap, task.Name, "", task.Opts.StartTaskOnCreate, task.Opts.MaxFailure)

	if result.Err == nil {
		return result.ID, nil
	}
	return "", errors.New("Create Task Failed: " + result.Err.Error())
}
