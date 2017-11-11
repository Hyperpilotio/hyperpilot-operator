package snap

import (
	"errors"
	"strconv"
	"time"

	"github.com/intelsdi-x/snap/mgmt/rest/client"
	"github.com/intelsdi-x/snap/scheduler/wmap"
)

const (
	SNAP_VERSION                = "v1"
	PROMETHEUS_TASK_NAME_PREFIX = "PROMETHEUS"
)

type Plugin struct {
	Name    string
	Type    string
	Version int
}

type RunOpts struct {
	ScheduleType      string
	ScheduleInterval  string
	StartTaskOnCreate bool
	MaxFailure        int
}

type TaskManager struct {
	*client.Client
	plugins []*Plugin
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
		Client:  snapClient,
		plugins: NewPrometheusPluginsList(),
	}, nil
}

func NewPrometheusPluginsList() []*Plugin {
	return []*Plugin{
		{"snap-plugin-collector-prometheus", "collector", 1},
		{"influxdb", "publisher", 22},
	}
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

func NewPrometheusCollectorWorkflowMap(podName string, namespace string, port int32) *wmap.WorkflowMap {
	ns := "/hyperpilot/prometheus"
	metrics := make(map[string]int)
	metrics["/hyperpilot/prometheus/"] = 1
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
	publisher := NewPublisher("influxdb", 22, publisherConfig)

	return publisher.JoinCollector(collector).Join(wmap.NewWorkflowMap())
}

func (manager *TaskManager) CreateTask(task *Task) (string, error) {
	sch := &client.Schedule{Type: task.Opts.ScheduleType, Interval: task.Opts.ScheduleInterval}
	var err error

	// Retry 5 times, TODO: Configurable
	for i := 0; i < 5; i++ {
		result := manager.Client.CreateTask(sch, task.WorkflowMap, task.Name, "", task.Opts.StartTaskOnCreate, task.Opts.MaxFailure)
		if result.Err == nil {
			return result.ID, nil
		} else {
			err = result.Err
		}

		time.Sleep(time.Second * 10)
	}

	return "", errors.New("Create Task Failed: " + err.Error())
}

func (manager *TaskManager) isPluginLoaded(plugin *Plugin) bool {
	r := manager.GetPlugin(plugin.Type, plugin.Name, plugin.Version)
	if r.Err != nil {
		return false
	}
	return true
}

func (manager *TaskManager) StopAndRemoveTask(taskId string) error {
	if r1 := manager.StopTask(taskId); r1.Err != nil {
		return r1.Err
	}

	if r2 := manager.RemoveTask(taskId); r2.Err != nil {
		return r2.Err
	}

	return nil
}

func (manager *TaskManager) IsReady() bool {
	for _, p := range manager.plugins {
		if manager.isPluginLoaded(p) == false {
			return false
		}
	}
	return true
}
