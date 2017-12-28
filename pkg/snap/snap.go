package snap

import (
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/intelsdi-x/snap/mgmt/rest/client"
	"github.com/intelsdi-x/snap/scheduler/wmap"
	"github.com/spf13/viper"
)

const (
	snapVersion              = "v1"
	prometheusTaskNamePrefix = "PROMETHEUS"
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

func NewTaskManager(podIP string, config *viper.Viper) (*TaskManager, error) {
	url := "http://" + podIP + ":8181"
	snapClient, err := client.New(url, snapVersion, true)
	if err != nil {
		return nil, errors.New("Unable to create snap client: " + err.Error())
	}

	return &TaskManager{
		Client:  snapClient,
		plugins: NewPrometheusPluginsList(config),
	}, nil
}

func NewPrometheusPluginsList(config *viper.Viper) []*Plugin {
	var plugins []*Plugin
	config.UnmarshalKey("SnapTaskController.PluginsList", &plugins)
	return plugins
}

func NewPrometheusCollectorTask(podName string, namespace string, port int32, config *viper.Viper) *Task {
	runOpts := &RunOpts{
		ScheduleType:      "simple",
		ScheduleInterval:  "5s",
		StartTaskOnCreate: true,
		MaxFailure:        -1,
	}

	return &Task{
		// e.g. PROMETHEUS-resource-worker-spark-9br5d
		Name:        prometheusTaskNamePrefix + "-" + podName,
		WorkflowMap: NewPrometheusCollectorWorkflowMap(podName, namespace, port, config),
		Opts:        runOpts,
	}
}

func NewPrometheusCollectorWorkflowMap(podName string, namespace string, port int32, conf *viper.Viper) *wmap.WorkflowMap {
	ns := "/hyperpilot/prometheus"
	metrics := make(map[string]int)
	metrics["/hyperpilot/prometheus/"] = 1
	config := make(map[string]interface{})
	config["endpoint"] = "http://" + podName + "." + namespace + ":" + strconv.Itoa(int(port))
	collector := NewCollector(ns, metrics, config)

	// create influx publisher
	publisherConfig := make(map[string]interface{})
	publisherConfig["host"] = conf.GetString("SnapTaskController.influxdb.host")
	publisherConfig["port"] = conf.GetInt("SnapTaskController.influxdb.port")
	publisherConfig["database"] = conf.GetString("SnapTaskController.influxdb.database")
	publisherConfig["user"] = conf.GetString("SnapTaskController.influxdb.user")
	publisherConfig["password"] = conf.GetString("SnapTaskController.influxdb.password")
	publisherConfig["https"] = conf.GetBool("SnapTaskController.influxdb.https")
	publisherConfig["skip-verify"] = conf.GetBool("SnapTaskController.influxdb.skip-verify")
	publisher := NewPublisher("influxdb", 22, publisherConfig)

	return publisher.JoinCollector(collector).Join(wmap.NewWorkflowMap())
}

func (manager *TaskManager) CreateTask(task *Task, config *viper.Viper) (string, error) {
	sch := &client.Schedule{Type: task.Opts.ScheduleType, Interval: task.Opts.ScheduleInterval}
	var err error
	retryCount := config.GetInt("SnapTaskController.CreateTaskRetry")

	for i := 0; i < retryCount; i++ {
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
		log.Printf("[ TaskManager ] Plugins download not ready: %s", r.Err.Error())
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

func (manager *TaskManager) isReady() bool {
	for _, p := range manager.plugins {
		if manager.isPluginLoaded(p) == false {
			return false
		}
	}
	return true
}

func (manager *TaskManager) WaitForLoadPlugins(min int) error {
	timeout := time.After(time.Duration(min) * time.Minute)
	tick10s := time.Tick(10 * time.Second)
	tick := time.Tick(5 * time.Second)
	for {
		select {
		case <-timeout:
			return errors.New("Plugin download timeout")
		case <-tick10s:
			log.Printf("[ TaskManager ] Wait for loading plugin complete")
		case <-tick:
			if manager.isReady() {
				return nil
			}
		}
	}
}
