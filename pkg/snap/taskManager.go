package snap

import (
	"errors"

	"github.com/intelsdi-x/snap/mgmt/rest/client"
	"github.com/intelsdi-x/snap/scheduler/wmap"
)

type RunOpts struct {
	ScheduleType      string
	ScheduleInterval  string
	StartTaskOnCreate bool
	MaxFailure        int
}

type Task struct {
	Name        string
	WorkflowMap *wmap.WorkflowMap
	Opts        *RunOpts
}

func NewTask(name string, workflowMap *wmap.WorkflowMap, opts ...*RunOpts) (*Task, error) {
	var runOpts *RunOpts
	if len(opts) > 1 {
		return nil, errors.New("RunOpts should between 0..1")
	}
	if len(opts) == 0 {
		runOpts = &RunOpts{
			ScheduleType:      "simple",
			ScheduleInterval:  "5s",
			StartTaskOnCreate: true,
			MaxFailure:        -1,
		}
	} else {
		runOpts = opts[0]
	}
	return &Task{
		Name:        name,
		WorkflowMap: workflowMap,
		Opts:        runOpts,
	}, nil
}

type SnapTaskManager struct {
	*client.Client
}

func NewSnapTaskManager(url, version string) (*SnapTaskManager, error) {
	snapClient, err := client.New(url, version, true)

	if err != nil {
		return nil, err
	}
	return &SnapTaskManager{
		Client: snapClient,
	}, nil
}

func (manager *SnapTaskManager) CreateTask(task *Task) (taskId string, err error) {
	sch := &client.Schedule{Type: task.Opts.ScheduleType, Interval: task.Opts.ScheduleInterval}
	result := manager.Client.CreateTask(sch, task.WorkflowMap, task.Name, "", task.Opts.StartTaskOnCreate, task.Opts.MaxFailure)
	return result.ID, result.Err
}
