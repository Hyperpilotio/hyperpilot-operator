package snap

import (
	"github.com/intelsdi-x/snap/mgmt/rest/client"
	"github.com/intelsdi-x/snap/scheduler/wmap"
)

const (
	SNAP_VERSION = "v1"
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
		Name:        podName + "-Prometheus",
		WorkflowMap: NewWorkflowMap(podName),
		Opts:        runOpts,
	}
}


func NewWorkflowMap(name string)*wmap.WorkflowMap{

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

//todo define task
func NewWorkflowMap1(resourcePodName string) *wmap.WorkflowMap {
	// Create collector
	prometheusConfig := make(map[string]interface{})
	// todo check url format or use pod ip
	//prometheusConfig["endpoint"] = "http://" + resourcePodName + ".resource-worker:7998"
	//prometheusConfig["endpoint"] = "http://resource-worker-0.resource-worker:7998"
	prometheusConfig["procfs"] = "/proc_host"

	metrics := make(map[string]int)
	metrics["/intel/docker//stats/cgroups/"] = 1
	prometheusCollector := NewCollector("/intel/docker", metrics, prometheusConfig)

	// create processor
	processorConfig := make(map[string]interface{})
	processorConfig["collect.namespaces"] = "default"
	processorConfig["collect.include_empty_namespace"] = true
	processorConfig["collect.exclude_metrics"] = "cpu_stats/cpu_shares, /cpuset_stats, /pids_stats/, /cpu_usage/per_cpu/*"
	processorConfig["collect.exclude_metrics.except"] = ""
	processorConfig["average"] =
		"/blkio_stats/, " +
			"/cpu_usage/, " +
			"/cpu_stats/throttling_data/, " +
			"intel/docker/stats/cgroups/memory_stats/usage/failcnt, " +
			"intel/docker/stats/cgroups/memory_stats/usage/pgfault, " +
			"intel/docker/stats/cgroups/memory_stats/usage/pgmajfault, " +
			"intel/docker/stats/cgroups/memory_stats/usage/pgpgin, " +
			"intel/docker/stats/cgroups/memory_stats/usage/pgpgout, " +
			"intel/docker/stats/cgroups/memory_stats/usage/total_pgfault, " +
			"intel/docker/stats/cgroups/memory_stats/usage/total_pgmajfault, " +
			"intel/docker/stats/cgroups/memory_stats/usage/total_pgppin, " +
			"intel/docker/stats/cgroups/memory_stats/usage/total_pgpgout, " +
			"intel/docker/stats/cgroups/hugetlb_stats/usage/failcnt"

	averageProcessor := NewProcessor("snap-average-counter-processor", 1, processorConfig)

	// create publisher
	publisherConfig := make(map[string]interface{})
	publisherConfig["host"] = "influxsrv"
	publisherConfig["port"] = 8086
	publisherConfig["database"] = "snapaverage"
	publisherConfig["user"] = "root"
	publisherConfig["password"] = "default"
	publisherConfig["https"] = false
	publisherConfig["skip-verify"] = false
	influxPublisher := NewPublisher("influxdb", 1, publisherConfig)

	// compose all these component
	wf := influxPublisher.JoinProcessor(averageProcessor).Join(prometheusCollector).Join(wmap.NewWorkflowMap())

	return wf
}

func (manager *TaskManager) CreateTask(task *Task) (taskId string, err error) {
	sch := &client.Schedule{Type: task.Opts.ScheduleType, Interval: task.Opts.ScheduleInterval}
	result := manager.Client.CreateTask(sch, task.WorkflowMap, task.Name, "", task.Opts.StartTaskOnCreate, task.Opts.MaxFailure)
	return result.ID, result.Err
}
