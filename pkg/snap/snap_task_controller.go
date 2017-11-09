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
	NodeId      string
	ExternalIP  string
	TaskManager *TaskManager

	// servicePodName <-> Task name
	SnapTasks map[string]string

	// servicePodName <-> pod info
	RunningServicePods map[string]ServicePodInfo

	PodEvents chan *operator.PodEvent
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
	return operator.POD | operator.NODE
}

func (node *SnapNode) reconcileSnapState() error {
	// If any pod in runningServicePods doesn't exist in SnapTasks value,
	// Create a new snap task for that new pod
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

	// If any tasks in SnapTasks points to a pod not in the running service pods list,
	// Delete the task from that snap.
	for servicePodName, taskName := range node.SnapTasks {
		_, ok := node.RunningServicePods[servicePodName]
		if !ok {
			taskId, err := getTaskIDFromTaskName(node.TaskManager, taskName)
			if err != nil {
				return err
			}
			node.TaskManager.StopAndRemoveTask(taskId)
			// TODO: Check result!
			delete(node.SnapTasks, servicePodName)
			log.Printf("Delete task {%s} in Node {%s}", taskName, node.NodeId)
		}
	}

	return nil
}

func (n *SnapNode) Run(isOutsideCluster bool) {
	go func() {
		for e := range n.PodEvents {
			switch e.EventType {
			case operator.ADD, operator.UPDATE:
				// Check if it's running and is part of a service we're looking for
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
					// delete pod from nodeInfo.RunningServicePods
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
	}()
}

func (s *SnapTaskController) reconcileSnapState() bool {
	// Check each node reconcileSnapState
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

func (s *SnapTaskController) Init(clusterState *operator.ClusterState) error {
	s.ClusterState = clusterState

	// Initialize steps:
	// 1. Create SnapNode for each node, TaskManager default is nil
	for _, n := range clusterState.Nodes {
		snapNode := &SnapNode{
			NodeId:             n.NodeName,
			ExternalIP:         n.ExternalIP,
			RunningServicePods: make(map[string]ServicePodInfo),
			SnapTasks:          make(map[string]string),
			TaskManager:        nil,
			PodEvents:          make(chan *operator.PodEvent),
		}
		init, err := snapNode.init(s.isOutsideCluster, clusterState)
		if !init {
			log.Printf("Snap is not found in the cluster for node during init: %s", n.NodeName)
			// We will assume a new snap will be running and we will be notified at ProcessPod
		} else if err != nil {
			log.Printf("Unable to init snap for node %s: %s", n.NodeName, err.Error())
		} else {
			snapNode.Run(s.isOutsideCluster)
		}

		s.Nodes[n.NodeName] = snapNode
	}

	for _, p := range clusterState.Pods {
		if s.isServicePod(p) {
			// 2. Get all running service pods
			// TODO: How do we know which container has the right port? and which port?
			container := p.Spec.Containers[0]
			if len(container.Ports) > 0 {
				s.Nodes[p.Spec.NodeName].RunningServicePods[p.Name] = ServicePodInfo{
					Namespace: p.Namespace,
					Port:      container.Ports[0].HostPort,
				}
			}
		}
	}

	//s.printOut()
	return nil
}

func (n *SnapNode) initSnap(isOutsideCluster bool, snapPod *v1.Pod) error {
	var taskManager *TaskManager
	var err error
	if isOutsideCluster {
		// If run outside cluster, task manager communicate with snap with snap HOST IP
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

	// Get all running tasks from Snap API for existing snap pod
	tasks := n.TaskManager.GetTasks()
	for _, task := range tasks.ScheduledTasks {
		// only look for task which controller care about
		if isSnapControllerTask(task.Name) {
			servicePodName := getServicePodNameFromSnapTask(task.Name)
			n.SnapTasks[servicePodName] = task.Name
		}
	}

	n.reconcileSnapState()

	return nil
}

func (n *SnapNode) init(isOutsideCluster bool, clusterState *operator.ClusterState) (bool, error) {
	for _, p := range clusterState.Pods {
		if n.NodeId == p.Spec.NodeName && strings.HasPrefix(p.Name, "snap-") {
			if err := n.initSnap(isOutsideCluster, p); err != nil {
				return true, err
			}
			return true, nil
		}
	}

	log.Printf("Snap is not found in cluster for node %s during init", n.NodeId)
	return false, nil
}

// Task name format PROMETHEUS-<POD_NAME>
// e.g. PROMETHEUS-resource-worker-spark-9br5d
func getServicePodNameFromSnapTask(taskName string) string {
	return strings.SplitN(taskName, "-", 2)[1]
}

//we only want task name start with PROMETHEUS_TASK_NAME_PREFIX
func isSnapControllerTask(taskName string) bool {
	if strings.HasPrefix(taskName, PROMETHEUS_TASK_NAME_PREFIX) {
		return true
	}
	return false
}

func (s *SnapTaskController) ProcessNode(e *operator.NodeEvent) {
	switch e.EventType {
	case operator.ADD:
		node, ok := s.ClusterState.Nodes[e.Cur.Name]
		if !ok {
			// Somehow operator don't have the node?
		}
		s.Nodes[node.NodeName] = &SnapNode{
			NodeId:             node.NodeName,
			ExternalIP:         node.ExternalIP,
			RunningServicePods: make(map[string]ServicePodInfo),
			SnapTasks:          make(map[string]string),
			TaskManager:        nil,
		}
	case operator.DELETE:
	case operator.UPDATE:
	}
}

func (s *SnapTaskController) String() string {
	return fmt.Sprintf("SnapTaskController")
}

func (s *SnapTaskController) isServicePod(pod *v1.Pod) bool {
	for _, service := range s.ServiceList {
		if strings.HasPrefix(pod.Name, service) {
			return true
		}
	}

	return false
}

func (s *SnapTaskController) ProcessPod(e *operator.PodEvent) {
	// For ADD and DELETE event, just put into node.PodEvents
	// For Update, have to  check  Old.Status == "Pending" && Cur.Status == "Running"

	//SNAP pod event handled first in PorcessPod, other push to separated channel
	switch e.EventType {
	case operator.ADD:
		log.Printf("ADD in ProcessedPod")
		if e.Cur.Status.Phase == "Running" {
			nodeName := e.Cur.Spec.NodeName
			node, ok := s.Nodes[nodeName]
			if !ok {
				log.Printf("Received an unknown node from pod ADD event: %s", nodeName)
				return
			}

			if strings.HasPrefix(e.Cur.Name, "snap-") {
				return
			}

			if s.isServicePod(e.Cur) {
				node.PodEvents <- e
			}
		}
	case operator.DELETE:
		log.Printf("DELETE in ProcessedPod")

		nodeName := e.Cur.Spec.NodeName
		node, ok := s.Nodes[nodeName]
		if !ok {
			log.Printf("Received an unknown node from DELETE pod event: %s", nodeName)
			return
		}

		if strings.HasPrefix(e.Cur.Name, "snap-") {
			node.SnapTasks = make(map[string]string)
			return
		}

		if s.isServicePod(e.Cur) {
			node.PodEvents <- e
		}
	case operator.UPDATE:
		log.Printf("UPDATE in ProcessedPod")

		if e.Old.Status.Phase == "Pending" && e.Cur.Status.Phase == "Running" {
			nodeName := e.Cur.Spec.NodeName
			node, ok := s.Nodes[nodeName]
			if !ok {
				log.Printf("Received an unknown node from pod UPDATE event: %s", nodeName)
				return
			}

			if strings.HasPrefix(e.Cur.Name, "snap-") {
				node.PodEvents = make(chan *operator.PodEvent)
				if err := node.initSnap(s.isOutsideCluster, e.Cur); err != nil {
					log.Printf("Unable to init snap during process pod for node %s: %s", nodeName, err.Error())
				}
				node.Run(s.isOutsideCluster)
				return
			}

			if s.isServicePod(e.Cur) {
				node.PodEvents <- e
			}
		}
	}
}

func (s *SnapTaskController) ProcessDeployment(e *operator.DeploymentEvent) {
	switch e.EventType {
	case operator.ADD:
	case operator.DELETE:
	case operator.UPDATE:
	}
}

func (s *SnapTaskController) ProcessDaemonSet(e *operator.DaemonSetEvent) {
	switch e.EventType {
	case operator.ADD:
	case operator.DELETE:
	case operator.UPDATE:
	}
}

func (s *SnapTaskController) printOut() {
	log.Printf("================================")
	for k, v := range s.Nodes {
		log.Printf("Controller info: {NodeName, Info}: {%s, %s}", k, v)
	}

	for _, node := range s.Nodes {
		taskInNode := make([]string, 0, 0)

		for _, task := range node.SnapTasks {
			taskInNode = append(taskInNode, task)
		}
		log.Printf("Controller info: {node, task}: {%s, %s}", node.NodeId, taskInNode)
	}

	for _, node := range s.Nodes {
		serviceInNode := make([]string, 0, 0)
		for k, _ := range node.RunningServicePods {
			serviceInNode = append(serviceInNode, k)
		}
		log.Printf("Controller info : {node, runningPod}: {%s, %s}", node.NodeId, serviceInNode)
	}
	log.Printf("================================")

}
