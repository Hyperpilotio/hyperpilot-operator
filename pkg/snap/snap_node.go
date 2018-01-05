package snap

import (
	"log"
	"strconv"
	"sync"

	"errors"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/spf13/viper"
	"k8s.io/client-go/pkg/api/v1"
	"strings"
)

type ServicePodInfo struct {
	Namespace string
	Port      int32
	PodIP     string
	PodName   string
	AppID     string
}

func (s *ServicePodInfo) buildPrometheusMetricURL() string {
	return "http://" + s.PodIP + ":" + strconv.Itoa(int(s.Port))
}

type RunningServiceList struct {
	Lock               *sync.Mutex
	RunningServicePods map[string]ServicePodInfo
	SnapNode           *SnapNode
}

func NewRunningServiceList(node *SnapNode) *RunningServiceList {
	return &RunningServiceList{
		Lock:               &sync.Mutex{},
		RunningServicePods: make(map[string]ServicePodInfo),
		SnapNode:           node,
	}
}

func (runningService *RunningServiceList) reconcile() error {
	runningService.Lock.Lock()
	defer runningService.Lock.Unlock()

	for servicePodName, podInfo := range runningService.RunningServicePods {
		if err := runningService.SnapNode.SnapTasks.createTask(podInfo); err != nil {
			log.Printf("[ RunningServiceList ] Create task fail when snap task for {%s} is not exist: %s", servicePodName, err.Error())
			return err
		}
	}
	return nil
}

func (runningService *RunningServiceList) find(servicePodName string) bool {
	runningService.Lock.Lock()
	defer runningService.Lock.Unlock()
	_, ok := runningService.RunningServicePods[servicePodName]
	return ok
}

func (runningService *RunningServiceList) addPodInfo(name string, podInfo ServicePodInfo) {
	runningService.Lock.Lock()
	defer runningService.Lock.Unlock()
	runningService.RunningServicePods[name] = podInfo
	log.Printf("[ RunningServiceList ] Add Service Pod {%s}", name)
}

func (runningService *RunningServiceList) delPodInfo(name string) {
	runningService.Lock.Lock()
	defer runningService.Lock.Unlock()

	if _, ok := runningService.RunningServicePods[name]; ok {
		delete(runningService.RunningServicePods, name)
		log.Printf("[ RunningServiceList ] Delete Service Pod {%s}", name)
	} else {
		log.Printf("[ RunningServiceList ] Unable to find and delete service pod {%s} in running tasks!", name)
	}
}

func (runningService *RunningServiceList) deletePodInfoIfNotPresentInList(podList []*v1.Pod) {
	runningService.Lock.Lock()
	defer runningService.Lock.Unlock()

	for _, p := range podList {
		_, ok := runningService.RunningServicePods[p.Name]
		if !ok {
			delete(runningService.RunningServicePods, p.Name)
			log.Printf("[ RunningServiceList ] Delete Running Service Pod {%s}", p.Name)
		}
	}
}

type SnapTaskList struct {
	Lock      *sync.Mutex
	SnapTasks map[string]string
	SnapNode  *SnapNode
}

func NewSnapTaskList(node *SnapNode) *SnapTaskList {
	return &SnapTaskList{
		Lock:      &sync.Mutex{},
		SnapTasks: make(map[string]string),
		SnapNode:  node,
	}
}

func (snapTask *SnapTaskList) createTask(podInfo ServicePodInfo) error {
	snapTask.Lock.Lock()
	defer snapTask.Lock.Unlock()

	n := snapTask.SnapNode
	_, ok := snapTask.SnapTasks[podInfo.PodName]
	if !ok {
		task := NewPrometheusCollectorTask(podInfo, n.config)
		_, err := n.TaskManager.CreateTask(task, n.config)
		if err != nil {
			log.Printf("[ SnapTaskList ] Snap create task fail: %s", err.Error())
			return err
		}
		snapTask.SnapTasks[podInfo.PodName] = task.Name
		log.Printf("[ SnapTaskList ] Snap Create task {%s}", task.Name)
	}

	return nil
}

func (snapTask *SnapTaskList) addTaskName(servicePodName, taskName string) {
	snapTask.Lock.Lock()
	defer snapTask.Lock.Unlock()
	snapTask.SnapTasks[servicePodName] = taskName
}

func (snapTask *SnapTaskList) reconcile() error {
	snapTask.Lock.Lock()
	defer snapTask.Lock.Unlock()
	n := snapTask.SnapNode

	for servicePodName, taskName := range snapTask.SnapTasks {
		ok := n.RunningServicePods.find(servicePodName)
		if !ok {
			taskId, err := getTaskIDFromTaskName(n.TaskManager, taskName)
			if err != nil {
				log.Printf("[ SnapTaskList ] Get id of task {%s} fail when deleting: %s", taskName, err.Error())
				return err
			}
			err = n.TaskManager.StopAndRemoveTask(taskId)
			if err != nil {
				log.Printf("[ SnapTaskList ] Delete task {%s} fail: %s", taskName, err.Error())
				return err
			}
			delete(snapTask.SnapTasks, servicePodName)
			log.Printf("[ SnapTaskList ] Delete task {%s}", taskName)
		}
	}
	return nil
}

type SnapNode struct {
	Host               string
	TaskManager        *TaskManager
	SnapTasks          *SnapTaskList
	RunningServicePods *RunningServiceList
	PodEvents          chan *common.PodEvent
	ExitChan           chan bool
	ServiceList        *ServiceWatchingList
	config             *viper.Viper
}

func NewSnapNode(host string, serviceList *ServiceWatchingList, config *viper.Viper) *SnapNode {
	snapNode := &SnapNode{
		Host:        host,
		TaskManager: nil,
		PodEvents:   make(chan *common.PodEvent, 1000),
		ExitChan:    make(chan bool, 1),
		ServiceList: serviceList,
		config:      config,
	}
	runningServices := NewRunningServiceList(snapNode)
	snapTasks := NewSnapTaskList(snapNode)
	snapNode.RunningServicePods = runningServices
	snapNode.SnapTasks = snapTasks
	return snapNode
}

func (n *SnapNode) initSingleSnap(clusterState *common.ClusterState) error {
	var taskManager *TaskManager
	var err error
	taskManager, err = NewTaskManager(n.Host, n.config)

	if err != nil {
		log.Printf("Failed to create Snap Task Manager: " + err.Error())
		return err
	}

	n.TaskManager = taskManager

	//todo: how to initSnap if timeout
	err = n.TaskManager.WaitForLoadPlugins(n.config.GetInt("SnapTaskController.PluginDownloadTimeoutMin"))
	if err != nil {
		log.Printf("Failed to create Snap Task Manager: {%s}", err.Error())
		return err
	}
	log.Printf("[ SnapNode ] Load plugins completed")

	tasks := n.TaskManager.GetTasks()
	for _, task := range tasks.ScheduledTasks {
		if isSnapControllerTask(task.Name) {
			servicePodName := getServicePodNameFromSnapTask(task.Name)
			n.SnapTasks.addTaskName(servicePodName, task.Name)
		}
	}

	clusterState.Lock.RLock()
	for _, p := range clusterState.Pods {
		if n.ServiceList.isServicePod(p) {
			// TODO: How do we know which container has the right port? and which port?
			container := p.Spec.Containers[0]
			appID := n.ServiceList.findAppIDOfMicroservice(p.Name)
			if len(container.Ports) > 0 {
				n.RunningServicePods.addPodInfo(p.Name, ServicePodInfo{
					Namespace: p.Namespace,
					Port:      container.Ports[0].HostPort,
					PodIP:     p.Status.PodIP,
					PodName:   p.Name,
					AppID:     appID,
				})
			}
		}
	}
	clusterState.Lock.RUnlock()

	n.reconcileSnapState()
	n.Run()
	return nil
}

func (n *SnapNode) reconcileSnapState() error {
	// create snap task for existing running service
	if err := n.RunningServicePods.reconcile(); err != nil {
		log.Printf("[ SnapNode ] reconcileSnapState fail when check running service to create task : %s", err.Error())
		return err
	}
	// remove snap task for non-existing running service
	if err := n.SnapTasks.reconcile(); err != nil {
		log.Printf("[ SnapNode ] reconcileSnapState fail when check SnapTask list to remove task : %s", err.Error())
		return err
	}
	return nil
}

func (n *SnapNode) Run() {
	go func() {
		for {
			select {
			case <-n.ExitChan:
				log.Printf("[ SnapNode ] Snap is signaled to exit")
				return
			case e := <-n.PodEvents:
				switch e.EventType {
				case common.ADD, common.UPDATE:
					appId := n.ServiceList.findAppIDOfMicroservice(e.Cur.Name)
					container := e.Cur.Spec.Containers[0]
					n.RunningServicePods.addPodInfo(e.Cur.Name, ServicePodInfo{
						Namespace: e.Cur.Namespace,
						Port:      container.Ports[0].HostPort,
						PodIP:     e.Cur.Status.PodIP,
						PodName:   e.Cur.Name,
						AppID:     appId,
					})
					if err := n.reconcileSnapState(); err != nil {
						log.Printf("[ SnapNode ] Unable to reconcile snap state: %s", err.Error())
						return
					}
					log.Printf("[ SnapNode ] Insert Service Pod {%s}", e.Cur.Name)

				case common.DELETE:
					n.RunningServicePods.delPodInfo(e.Cur.Name)
					if err := n.reconcileSnapState(); err != nil {
						log.Printf("[ SnapNode ] Unable to reconcile snap state: %s", err.Error())
						return
					}
				}
			}
		}
	}()
}

func (n *SnapNode) Exit() {
	n.ExitChan <- true
}

func isSnapControllerTask(taskName string) bool {
	if strings.HasPrefix(taskName, prometheusTaskNamePrefix) {
		return true
	}
	return false
}

func getServicePodNameFromSnapTask(taskName string) string {
	return strings.SplitN(taskName, "-", 2)[1]
}

func getTaskIDFromTaskName(manager *TaskManager, name string) (string, error) {
	tasks := manager.GetTasks().ScheduledTasks
	for _, t := range tasks {
		if t.Name == name {
			return t.ID, nil
		}
	}
	return "", errors.New("Can not find ID from Name")
}
