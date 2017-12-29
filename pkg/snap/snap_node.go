package snap

import (
	"log"
	"strings"
	"sync"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/spf13/viper"
	"k8s.io/client-go/pkg/api/v1"
)

type ServicePodInfo struct {
	Namespace string
	Port      int32
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
		if err := runningService.SnapNode.SnapTasks.createTask(servicePodName, podInfo); err != nil {
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

	for podName := range runningService.RunningServicePods {
		if !containPod(podList, podName) {
			delete(runningService.RunningServicePods, podName)
			log.Printf("[ RunningServiceList ] Delete Running Service Pod {%s} in Node {%s} ", podName, runningService.SnapNode.NodeId)
		}
	}
}

func containPod(pods []*v1.Pod, podName string) bool {
	for _, pod := range pods {
		if podName == pod.Name {
			return true
		}
	}
	return false
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

func (snapTask *SnapTaskList) createTask(servicePodName string, podInfo ServicePodInfo) error {
	snapTask.Lock.Lock()
	defer snapTask.Lock.Unlock()

	n := snapTask.SnapNode
	_, ok := snapTask.SnapTasks[servicePodName]
	if !ok {
		task := NewPrometheusCollectorTask(servicePodName, podInfo.Namespace, podInfo.Port, n.config)
		_, err := n.TaskManager.CreateTask(task, n.config)
		if err != nil {
			log.Printf("[ SnapTaskList ] Create task in Node {%s} fail: %s", n.NodeId, err.Error())
			return err
		}
		snapTask.SnapTasks[servicePodName] = task.Name
		log.Printf("[ SnapTaskList ] Create task {%s} in Node {%s}", task.Name, n.NodeId)
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
				log.Printf("[ SnapTaskList ] Get id of task {%s} in Node {%s} fail when deleting: %s", taskName, n.NodeId, err.Error())
				return err
			}
			err = n.TaskManager.StopAndRemoveTask(taskId)
			if err != nil {
				log.Printf("[ SnapTaskList ] Delete task {%s} in Node {%s} fail: %s", taskName, n.NodeId, err.Error())
				return err
			}
			delete(snapTask.SnapTasks, servicePodName)
			log.Printf("[ SnapTaskList ] Delete task {%s} in Node {%s}", taskName, n.NodeId)
		}
	}
	return nil
}

type SnapNode struct {
	NodeId             string
	ExternalIP         string
	TaskManager        *TaskManager
	SnapTasks          *SnapTaskList
	RunningServicePods *RunningServiceList
	PodEvents          chan *common.PodEvent
	ExitChan           chan bool
	ServiceList        *[]string
	config             *viper.Viper
}

func NewSnapNode(nodeName string, externalIp string, serviceList *[]string, config *viper.Viper) *SnapNode {
	snapNode := &SnapNode{
		NodeId:      nodeName,
		ExternalIP:  externalIp,
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

func (n *SnapNode) init(isOutsideCluster bool, clusterState *common.ClusterState) (bool, error) {
	clusterState.Lock.RLock()
	defer clusterState.Lock.RUnlock()

	for _, p := range clusterState.Pods {
		if n.NodeId == p.Spec.NodeName && isSnapPod(p) {
			if err := n.initSnap(isOutsideCluster, p, clusterState); err != nil {
				return true, err
			}
			return true, nil
		}
	}
	log.Printf("Snap is not found in cluster for node %s during init", n.NodeId)
	return false, nil
}

func (n *SnapNode) initSingleSnap(isOutsideCluster bool, snapPod *v1.Pod, clusterState *common.ClusterState) error {
	var taskManager *TaskManager
	var err error
	if isOutsideCluster {
		taskManager, err = NewTaskManager(n.ExternalIP, n.config)
	} else {
		taskManager, err = NewTaskManager(snapPod.Status.PodIP, n.config)
	}

	if err != nil {
		log.Printf("Failed to create Snap Task Manager: " + err.Error())
		return err
	}

	n.TaskManager = taskManager

	//todo: how to initSnap if timeout
	err = n.TaskManager.WaitForLoadPlugins(n.config.GetInt("SnapTaskController.PluginDownloadTimeoutMin"))
	if err != nil {
		log.Printf("Failed to create Snap Task Manager in Node {%s}: {%s}", n.NodeId, err.Error())
		return err
	}
	log.Printf("[ SnapNode ] {%s} Loads Plugin  Complete", n.NodeId)

	tasks := n.TaskManager.GetTasks()
	for _, task := range tasks.ScheduledTasks {
		if isSnapControllerTask(task.Name) {
			servicePodName := getServicePodNameFromSnapTask(task.Name)
			n.SnapTasks.addTaskName(servicePodName, task.Name)
		}
	}

	clusterState.Lock.RLock()
	for _, p := range clusterState.Pods {
		if n.isServicePod(p) {
			// TODO: How do we know which container has the right port? and which port?
			container := p.Spec.Containers[0]
			if len(container.Ports) > 0 {
				n.RunningServicePods.addPodInfo(p.Name, ServicePodInfo{
					Namespace: p.Namespace,
					Port:      container.Ports[0].HostPort,
				})
			}
		}
	}
	clusterState.Lock.RUnlock()

	n.reconcileSnapState()
	n.Run(isOutsideCluster)
	return nil
}

func (n *SnapNode) initSnap(isOutsideCluster bool, snapPod *v1.Pod, clusterState *common.ClusterState) error {
	var taskManager *TaskManager
	var err error
	if isOutsideCluster {
		taskManager, err = NewTaskManager(n.ExternalIP, n.config)
	} else {
		taskManager, err = NewTaskManager(snapPod.Status.PodIP, n.config)
	}

	if err != nil {
		log.Printf("Failed to create Snap Task Manager: " + err.Error())
		return err
	}

	n.TaskManager = taskManager

	//todo: how to initSnap if timeout
	err = n.TaskManager.WaitForLoadPlugins(n.config.GetInt("SnapTaskController.PluginDownloadTimeoutMin"))
	if err != nil {
		log.Printf("Failed to create Snap Task Manager in Node {%s}: {%s}", n.NodeId, err.Error())
		return err
	}
	log.Printf("[ SnapNode ] {%s} Loads Plugin  Complete", n.NodeId)

	tasks := n.TaskManager.GetTasks()
	for _, task := range tasks.ScheduledTasks {
		if isSnapControllerTask(task.Name) {
			servicePodName := getServicePodNameFromSnapTask(task.Name)
			n.SnapTasks.addTaskName(servicePodName, task.Name)
		}
	}

	clusterState.Lock.RLock()
	for _, p := range clusterState.Pods {
		if p.Spec.NodeName == n.NodeId && n.isServicePod(p) {
			// TODO: How do we know which container has the right port? and which port?
			container := p.Spec.Containers[0]
			if len(container.Ports) > 0 {
				n.RunningServicePods.addPodInfo(p.Name, ServicePodInfo{
					Namespace: p.Namespace,
					Port:      container.Ports[0].HostPort,
				})
			}
		}
	}
	clusterState.Lock.RUnlock()

	n.reconcileSnapState()
	n.Run(isOutsideCluster)
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

func (n *SnapNode) Run(isOutsideCluster bool) {
	go func() {
		for {
			select {
			case <-n.ExitChan:
				log.Printf("[ SnapNode ] {%s} signaled to exit", n.NodeId)
				return
			case e := <-n.PodEvents:
				switch e.EventType {
				case common.ADD, common.UPDATE:
					container := e.Cur.Spec.Containers[0]
					n.RunningServicePods.addPodInfo(e.Cur.Name, ServicePodInfo{
						Namespace: e.Cur.Namespace,
						Port:      container.Ports[0].HostPort,
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

func (n *SnapNode) isServicePod(pod *v1.Pod) bool {
	for _, service := range *n.ServiceList {
		if strings.HasPrefix(pod.Name, service) {
			return true
		}
	}
	return false
}

func (n *SnapNode) Exit() {
	n.ExitChan <- true
}
