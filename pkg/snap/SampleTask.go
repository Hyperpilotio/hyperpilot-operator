package snap

import (
	"fmt"

	"github.com/intelsdi-x/snap/scheduler/wmap"
)

func SampleFunction() {

	// Create collector
	prometheusConfig := make(map[string]interface{})
	prometheusConfig["endpoint"] = "http://resource-worker-0.resource-worker:7998"

	metrics := make(map[string]int)
	metrics["/hyperpilot/prometheus"] = 1
	prometheusCollector := NewCollector("/hyperpilot/prometheus", metrics, prometheusConfig)

	// create processor
	processorConfig := make(map[string]interface{})
	processorConfig["collect.namespaces"] = "default"
	processorConfig["collect.include_empty_namespace"] = true
	processorConfig["collect.exclude_metrics"] = "*"
	processorConfig["collect.exclude_metrics.except"] = "*perc"
	processorConfig["average"] = ""
	averageProcessor := NewProcessor("snap-average-counter-processor", 1, processorConfig)

	// create publisher
	publisherConfig := make(map[string]interface{})
	publisherConfig["host"] = "influxsrv"
	publisherConfig["port"] = 8086
	publisherConfig["database"] = "snap"
	publisherConfig["user"] = "root"
	publisherConfig["password"] = "default"
	publisherConfig["https"] = false
	publisherConfig["skip-verify"] = false
	influxPublisher := NewPublisher("influxdb", 1, publisherConfig)

	// compose all these component
	wf := influxPublisher.JoinProcessor(averageProcessor).Join(prometheusCollector).Join(wmap.NewWorkflowMap())
	// same as
	// wf := prometheusCollector.PutProcessor(averageProcessor).PutPublisher(influxPublisher).Join(wmap.NewWorkflowMap())
	// same as
	// wf := averageProcessor.Put(influxPublisher).Join(prometheusCollector).Join(wmap.NewWorkflowMap())

	task, err := NewTask("sampleTask", wf)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	manager, err := NewSnapTaskManager("http://localhost:8181", "v1")
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	id, err := manager.CreateTask(task)
	fmt.Printf("ID: %v\n Error: %v", id, err)
}
