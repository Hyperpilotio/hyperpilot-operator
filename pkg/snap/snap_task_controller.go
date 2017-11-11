package snap

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/operator"
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
	SnapTasks          map[string]string
	RunningServicePods map[string]ServicePodInfo
	PodEvents          chan *operator.PodEvent
	ExitChan           chan bool
	ServiceList        []string
}

type SnapTaskController struct {
	isOutsideCluster bool
	ServiceList      []string
	Nodes            map[string]*SnapNode
	ClusterState     *operator.ClusterState
}

func NewSnapTaskController(runOutsideCluster bool) *SnapTaskController {
	return &SnapTaskController{
		ServiceList: []string{
			//There will be other type in the future
			"resource-worker",
		},
		Nodes:            map[string]*SnapNode{},
		isOutsideCluster: runOutsideCluster,
	}
}

func (s *SnapTaskController) GetResourceEnum() operator.ResourceEnum {
	//return POD | NODE | DAEMONSET
	return operator.POD | operator.NODE
}

func (s *SnapTaskController) Close() {

}

func (node *SnapNode) reconcileSnapState() error {
	for servicePodName, podInfo := range node.RunningServicePods {
		_, ok := node.SnapTasks[servicePodName]
		if !ok {
			task := NewPrometheusCollectorTask(servicePodName, podInfo.Namespace, podInfo.Port)
			_, err := node.TaskManager.CreateTask(task)
			if err != nil {
				return err
			}
			node.SnapTasks[servicePodName] = task.Name
			log.Printf("Create task {%s} in Node {%s}", task.Name, node.NodeId)
		}
	}

	for servicePodName, taskName := range node.SnapTasks {
		_, ok := node.RunningServicePods[servicePodName]
		if !ok {
			taskId, err := getTaskIDFromTaskName(node.TaskManager, taskName)
			if err != nil {
				return err
			}
			err = node.TaskManager.StopAndRemoveTask(taskId)
			if err != nil {
				return nil
			}
			delete(node.SnapTasks, servicePodName)
			log.Printf("Delete task {%s} in Node {%s}", taskName, node.NodeId)
		}
	}
	return nil
}

func (s *SnapTaskController) reconcileSnapState() bool {
	for _, node := range s.Nodes {
		node.reconcileSnapState()
	}
	return true
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

func NewSnapNode(nodeName string, externalIp string, serviceList []string) *SnapNode {
	return &SnapNode{
		NodeId:             nodeName,
		ExternalIP:         externalIp,
		RunningServicePods: make(map[string]ServicePodInfo),
		SnapTasks:          make(map[string]string),
		TaskManager:        nil,
		PodEvents:          make(chan *operator.PodEvent, 1000),
		ExitChan:           make(chan bool, 1),
		ServiceList:        serviceList,
	}
}

func (s *SnapTaskController) Init(clusterState *operator.ClusterState) error {
	s.ClusterState = clusterState
	for _, n := range clusterState.Nodes {
		for _, p := range clusterState.Pods {
			if p.Spec.NodeName == n.NodeName && isSnapPod(p) {
				snapNode := NewSnapNode(n.NodeName, n.ExternalIP, s.ServiceList)
				init, err := snapNode.init(s.isOutsideCluster, clusterState)
				if !init {
					log.Printf("Snap is not found in the cluster for node during init: %s", n.NodeName)
					// We will assume a new snap will be running and we will be notified at ProcessPod
				} else if err != nil {
					log.Printf("Unable to init snap for node %s: %s", n.NodeName, err.Error())
				} else {
					s.Nodes[n.NodeName] = snapNode
				}
			}
		}
	}
	return nil
}

func (n *SnapNode) init(isOutsideCluster bool, clusterState *operator.ClusterState) (bool, error) {
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

func (n *SnapNode) initSnap(isOutsideCluster bool, snapPod *v1.Pod, clusterState *operator.ClusterState) error {
	var taskManager *TaskManager
	var err error
	if isOutsideCluster {
		taskManager, err = NewTaskManager(n.ExternalIP)
	} else {
		taskManager, err = NewTaskManager(snapPod.Status.PodIP)
	}

	if err != nil {
		log.Printf("Failed to create Snap Task Manager: " + err.Error())
		return err
	}

	n.TaskManager = taskManager

	for !n.TaskManager.IsReady() {
		log.Println("wait for Snap Task Manager Load plugin complete")
		time.Sleep(time.Second * 10)
		// TODO: Wait until a limit before we throw error
	}
	log.Printf("Plugin Load Complete")

	tasks := n.TaskManager.GetTasks()
	for _, task := range tasks.ScheduledTasks {
		if isSnapControllerTask(task.Name) {
			servicePodName := getServicePodNameFromSnapTask(task.Name)
			n.SnapTasks[servicePodName] = task.Name
		}
	}

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

	n.reconcileSnapState()
	n.Run(isOutsideCluster)
	return nil
}

func (s *SnapTaskController) ProcessPod(e *operator.PodEvent) {
	switch e.EventType {
	case operator.DELETE:
		if e.Cur.Status.Phase != "Running" {
			return
		}

		nodeName := e.Cur.Spec.NodeName
		node, ok := s.Nodes[nodeName]
		if !ok {
			log.Printf("Received an unknown node from DELETE pod event: %s", nodeName)
			return
		}

		if isSnapPod(e.Cur) {
			log.Printf("Delete SNAP %s pod in ProcessPod", e.Cur.Name)

			node.Exit()
			delete(s.Nodes, nodeName)

			return
		}

		if s.isServicePod(e.Cur) {
			node.PodEvents <- e
		}
	case operator.ADD, operator.UPDATE:
		if e.Cur.Status.Phase == "Running" && (e.Old == nil || e.Old.Status.Phase == "Pending") {
			nodeName := e.Cur.Spec.NodeName
			node, ok := s.Nodes[nodeName]
			if !ok {
				if isSnapPod(e.Cur) {
					log.Printf("Add new SnapNode")
					newNode := NewSnapNode(nodeName, s.ClusterState.Nodes[nodeName].ExternalIP, s.ServiceList)
					s.Nodes[nodeName] = newNode
					go func() {
						if err := newNode.initSnap(s.isOutsideCluster, e.Cur, s.ClusterState); err != nil {
							// Handle snap node wasn't able to initialize
							// Most likely need to remove snap from nodes map.
						}
					}()
				}
				return
			}
			if node.isServicePod(e.Cur) {
				node.PodEvents <- e
			}
		}
	}
}

func (n *SnapNode) Run(isOutsideCluster bool) {
	go func() {
		for {
			select {
			case <-n.ExitChan:
				log.Printf("Snap node %s signaled to exit", n.NodeId)
				return
			case e := <-n.PodEvents:
				switch e.EventType {
				case operator.ADD, operator.UPDATE:
					container := e.Cur.Spec.Containers[0]
					n.RunningServicePods[e.Cur.Name] = ServicePodInfo{
						Namespace: e.Cur.Namespace,
						Port:      container.Ports[0].HostPort,
					}
					if err := n.reconcileSnapState(); err != nil {
						log.Printf("Unable to reconcile snap state: %s", err.Error())
						return
					}
					log.Printf("[ Controller ] Insert Pod {%s}", e.Cur.Name)

				case operator.DELETE:
					if _, ok := n.RunningServicePods[e.Cur.Name]; ok {
						delete(n.RunningServicePods, e.Cur.Name)
						if err := n.reconcileSnapState(); err != nil {
							log.Printf("Unable to reconcile snap state: %s", err.Error())
							return
						}
						log.Printf("[ Controller ] Delete Pod {%s}", e.Cur.Name)
					} else {
						log.Printf("Unable to find and delete service pod {%s} in running tasks!", e.Cur.Name)
					}
				}
			}
		}
	}()
}

func (node *SnapNode) Exit() {
	node.ExitChan <- true
}

func getServicePodNameFromSnapTask(taskName string) string {
	return strings.SplitN(taskName, "-", 2)[1]
}

func isSnapControllerTask(taskName string) bool {
	if strings.HasPrefix(taskName, PROMETHEUS_TASK_NAME_PREFIX) {
		return true
	}
	return false
}

func (s *SnapTaskController) String() string {
	return fmt.Sprintf("SnapTaskController")
}

func (node *SnapNode) isServicePod(pod *v1.Pod) bool {
	for _, service := range node.ServiceList {
		if strings.HasPrefix(pod.Name, service) {
			return true
		}
	}
	return false
}

func isSnapPod(pod *v1.Pod) bool {
	if strings.HasPrefix(pod.Name, "snap-") {
		return true
	}
	return false
}

func (s *SnapTaskController) ProcessDeployment(e *operator.DeploymentEvent) {}

func (s *SnapTaskController) ProcessDaemonSet(e *operator.DaemonSetEvent) {}

func (s *SnapTaskController) ProcessNode(e *operator.NodeEvent) {}
