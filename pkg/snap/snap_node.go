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

type SnapNode struct {
	NodeId             string
	ExternalIP         string
	TaskManager        *TaskManager
	snapTaskMx         *sync.Mutex
	SnapTasks          map[string]string
	runningPodsMx      *sync.Mutex
	RunningServicePods map[string]ServicePodInfo
	PodEvents          chan *common.PodEvent
	ExitChan           chan bool
	ServiceList        *[]string
	config             *viper.Viper
}

func NewSnapNode(nodeName string, externalIp string, serviceList *[]string, config *viper.Viper) *SnapNode {
	return &SnapNode{
		NodeId:             nodeName,
		ExternalIP:         externalIp,
		runningPodsMx:      &sync.Mutex{},
		RunningServicePods: make(map[string]ServicePodInfo),
		snapTaskMx:         &sync.Mutex{},
		SnapTasks:          make(map[string]string),
		TaskManager:        nil,
		PodEvents:          make(chan *common.PodEvent, 1000),
		ExitChan:           make(chan bool, 1),
		ServiceList:        serviceList,
		config:             config,
	}
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
			n.SnapTasks[servicePodName] = task.Name
		}
	}

	clusterState.Lock.RLock()
	for _, p := range clusterState.Pods {
		if n.isServicePod(p) {
			// TODO: How do we know which container has the right port? and which port?
			container := p.Spec.Containers[0]
			if len(container.Ports) > 0 {
				n.RunningServicePods[p.Name] = ServicePodInfo{
					Namespace: p.Namespace,
					Port:      container.Ports[0].HostPort,
				}
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
			n.SnapTasks[servicePodName] = task.Name
		}
	}

	clusterState.Lock.RLock()
	for _, p := range clusterState.Pods {
		if p.Spec.NodeName == n.NodeId && n.isServicePod(p) {
			// TODO: How do we know which container has the right port? and which port?
			container := p.Spec.Containers[0]
			if len(container.Ports) > 0 {
				n.RunningServicePods[p.Name] = ServicePodInfo{
					Namespace: p.Namespace,
					Port:      container.Ports[0].HostPort,
				}
			}
		}
	}
	clusterState.Lock.RUnlock()

	n.reconcileSnapState()
	n.Run(isOutsideCluster)
	return nil
}

func (n *SnapNode) reconcileSnapState() error {
	n.runningPodsMx.Lock()
	defer n.runningPodsMx.Unlock()
	n.snapTaskMx.Lock()
	defer n.snapTaskMx.Unlock()

	for servicePodName, podInfo := range n.RunningServicePods {
		_, ok := n.SnapTasks[servicePodName]
		if !ok {
			task := NewPrometheusCollectorTask(servicePodName, podInfo.Namespace, podInfo.Port, n.config)
			_, err := n.TaskManager.CreateTask(task, n.config)
			if err != nil {
				log.Printf("[ SnapNode ] Create task in Node {%s} fail: %s", n.NodeId, err.Error())
				return err
			}
			n.SnapTasks[servicePodName] = task.Name
			log.Printf("[ SnapNode ] Create task {%s} in Node {%s}", task.Name, n.NodeId)
		}
	}

	for servicePodName, taskName := range n.SnapTasks {
		_, ok := n.RunningServicePods[servicePodName]
		if !ok {
			taskId, err := getTaskIDFromTaskName(n.TaskManager, taskName)
			if err != nil {
				log.Printf("[ SnapNode ] Get id of task {%s} in Node {%s} fail when deleting: %s", taskName, n.NodeId, err.Error())
				return err
			}
			err = n.TaskManager.StopAndRemoveTask(taskId)
			if err != nil {
				log.Printf("[ SnapNode ] Delete task {%s} in Node {%s} fail: %s", taskName, n.NodeId, err.Error())
				return err
			}
			delete(n.SnapTasks, servicePodName)
			log.Printf("[ SnapNode ] Delete task {%s} in Node {%s}", taskName, n.NodeId)
		}
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
					n.runningPodsMx.Lock()
					n.RunningServicePods[e.Cur.Name] = ServicePodInfo{
						Namespace: e.Cur.Namespace,
						Port:      container.Ports[0].HostPort,
					}
					n.runningPodsMx.Unlock()
					if err := n.reconcileSnapState(); err != nil {
						log.Printf("Unable to reconcile snap state: %s", err.Error())
						return
					}
					log.Printf("[ SnapNode ] Insert Service Pod {%s}", e.Cur.Name)

				case common.DELETE:
					n.runningPodsMx.Lock()
					if _, ok := n.RunningServicePods[e.Cur.Name]; ok {
						delete(n.RunningServicePods, e.Cur.Name)
						log.Printf("[ SnapNode ] Delete Service Pod {%s}", e.Cur.Name)
					} else {
						log.Printf("[ SnapNode ] Unable to find and delete service pod {%s} in running tasks!", e.Cur.Name)
					}
					n.runningPodsMx.Unlock()

					if err := n.reconcileSnapState(); err != nil {
						log.Printf("Unable to reconcile snap state: %s", err.Error())
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
