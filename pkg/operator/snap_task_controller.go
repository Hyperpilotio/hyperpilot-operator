package operator

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/snap"
	"k8s.io/client-go/pkg/api/v1"
)

type ServicePodInfo struct {
	Namespace string
	Port      int32
}

type SnapNode struct {
	NodeId      string
	ExternalIP  string
	TaskManager *snap.TaskManager

	// servicePodName <-> Task name
	SnapTasks map[string]string

	// servicePodName <-> pod info
	RunningServicePods map[string]ServicePodInfo

	PodEvents chan *PodEvent
}

type SnapTaskController struct {
	isOutsideCluster bool
	ServiceList      []string
	Nodes            map[string]*SnapNode
	Operator         *HyperpilotOperator
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

func (s *SnapTaskController) GetResourceEnum() ResourceEnum {
	//return POD | NODE | DAEMONSET
	return POD | NODE
}

func (s *SnapTaskController) Close() {

}

func (node *SnapNode) reconcileSnapState() error {
	// If any pod in runningServicePods doesn't exist in SnapTasks value,
	// Create a new snap task for that new pod
	for servicePodName, podInfo := range node.RunningServicePods {
		_, ok := node.SnapTasks[servicePodName]
		if !ok {
			task := snap.NewPrometheusCollectorTask(servicePodName, podInfo.Namespace, podInfo.Port)
			_, err := node.TaskManager.CreateTask(task)
			if err != nil {
				return err
			}
			node.SnapTasks[servicePodName] = task.Name
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
		}
	}

	return nil
}

func (n *SnapNode) Run(isOutsideCluster bool) {
	go func() {
		for e := range n.PodEvents {
			switch e.EventType {
			case ADD:
			case DELETE:
				if _, ok := n.RunningServicePods[e.Cur.Name]; ok {
					// delete pod from nodeInfo.RunningServicePods
					delete(n.RunningServicePods, e.Cur.Name)
					n.reconcileSnapState()
					log.Printf("[ Controller ] Delete Pod {%s}", e.Cur.Name)
				} else {
					log.Printf("Unable to find and delete service pod {%s} in running tasks!", e.Cur.Name)
				}

			case UPDATE:
				// Only check scheduled pods
				if e.Old.Status.Phase == "Pending" && e.Cur.Status.Phase == "Running" {
					// Check if it's running and is part of a service we're looking for
					container := e.Cur.Spec.Containers[0]
					n.RunningServicePods[e.Cur.Name] = ServicePodInfo{
						Namespace: e.Cur.Namespace,
						Port:      container.Ports[0].HostPort,
					}
					if err := n.reconcileSnapState(); err != nil {
						log.Printf("Unable to reconcile snap state: %s", err.Error())
					}

					log.Printf("[ Controller ] Insert Pod {%s}", e.Cur.Name)
				}
			}
		}
	}()
}

//for each node {
// Compare node.SnapTasks and RunningServicePods
// If any pod in runningServicePods doesn't exist in SnapTasks value,
// 		Create a new snap task for that new pod
//		return true if reconcile Snap OK
// If any tasks in SnapTasks points to a pod not in the running service pods list,
// 		Delete the task from that snap.
//		return true if reconcile Snap OK
//}
func (s *SnapTaskController) reconcileSnapState() bool {
	// Check each node reconcileSnapState
	for _, node := range s.Nodes {
		node.reconcileSnapState()
	}
	return true
}

func getTaskIDFromTaskName(manager *snap.TaskManager, name string) (string, error) {
	tasks := manager.GetTasks().ScheduledTasks
	for _, t := range tasks {
		if t.Name == name {
			return t.ID, nil
		}
	}
	return "", errors.New("Can not find ID from Name")
}

func (s *SnapTaskController) Init(operator *HyperpilotOperator) error {
	// Initialize steps:
	// 1. Create SnapNode for each node, TaskManager default is nil
	for _, n := range operator.nodes {
		snapNode := &SnapNode{
			NodeId:             n.NodeName,
			ExternalIP:         n.ExternalIP,
			RunningServicePods: make(map[string]ServicePodInfo),
			SnapTasks:          make(map[string]string),
			TaskManager:        nil,
		}
		init, err := snapNode.init(s.isOutsideCluster, operator)
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

	for _, p := range operator.pods {
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

	s.printOut()

	return nil

	// 2. If possible, find snap pod for each node
	//     if snap exist, create TaskManager
	//     if snap no exist, leave it nil

	//for _, p := range operator.pods {
	//	if strings.HasPrefix(p.Name, "snap-") {
	//if s.Nodes[p.Spec.NodeName].TaskManager == nil {
	//	var taskmanager *snap.TaskManager
	//	var err error
	//	if s.isOutsideCluster {
	//		// If run outside cluster, task manager communicate with snap with snap HOST IP
	//		taskmanager, err = snap.NewTaskManager(s.Nodes[p.Spec.NodeName].ExternalIP)
	//	} else {
	//		// if inside k8s cluster, task manager communicate with snap with snap POD ip
	//		taskmanager, err = snap.NewTaskManager(p.Status.PodIP)
	//	}
	//
	//	if err != nil {
	//		log.Printf("Failed to create Snap Task Manager: " + err.Error())
	//	}
	//	s.Nodes[p.Spec.NodeName].TaskManager = taskmanager
	//}
	//	}
	//}

	// 3. Get all running tasks from Snap API for existing snap pod
	//for _, node := range s.Nodes {
	//	if node.TaskManager != nil {
	//		tasks := node.TaskManager.GetTasks()
	//		for _, task := range tasks.ScheduledTasks {
	//			// only look for task which controller care about
	//			if s.isSnapControllerTask(task.Name) {
	//				servicePodName := s.getServicePodNameFromSnapTask(task.Name)
	//				node.SnapTasks[servicePodName] = task.Name
	//			}
	//		}
	//	}
	//}

}

func (n *SnapNode) initSnap(isOutsideCluster bool, snapPod *v1.Pod) error {
	var taskManager *snap.TaskManager
	var err error
	if isOutsideCluster {
		// If run outside cluster, task manager communicate with snap with snap HOST IP
		taskManager, err = snap.NewTaskManager(n.ExternalIP)
	} else {
		taskManager, err = snap.NewTaskManager(snapPod.Status.PodIP)
	}

	if err != nil {
		log.Printf("Failed to create Snap Task Manager: " + err.Error())
		return err
	}

	n.TaskManager = taskManager

	for !taskManager.IsReady() {
		//todo: timeout
		log.Println("wait for Snap Task Manager Load plugin complete")
		//fmt.Println("wait for Snap Task Manager Load plugin complete")
		time.Sleep(time.Second * 10)
		// TODO: Wait until a limit before we throw error
	}

	// Get all running tasks from Snap API for existing snap pod
	tasks := taskManager.GetTasks()
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

func (n *SnapNode) init(isOutsideCluster bool, operator *HyperpilotOperator) (bool, error) {
	for _, p := range operator.pods {
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
	if strings.HasPrefix(taskName, snap.PROMETHEUS_TASK_NAME_PREFIX) {
		return true
	}
	return false
}

func (s *SnapTaskController) ProcessNode(e *NodeEvent) {
	switch e.EventType {
	case ADD:
		node, ok := s.Operator.nodes[e.Cur.Name]
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
	case DELETE:
	case UPDATE:
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

func (s *SnapTaskController) ProcessPod(e *PodEvent) {
	// process snap separately
	// If this is a snap pod, we need to update the snap client to point to the new Snap

	nodeName := e.Cur.Spec.NodeName
	node, ok := s.Nodes[nodeName]
	if !ok {
		log.Printf("Received an unknown node from pod event: %s", nodeName)
		return
	}

	if strings.HasPrefix(e.Cur.Name, "snap-") {
		switch e.EventType {
		case UPDATE:
			if e.Old.Status.Phase == "Pending" && e.Cur.Status.Phase == "Running" {

				// TODO: Handle snap restart after running on an existing node.
				if err := node.initSnap(s.isOutsideCluster, e.Cur); err != nil {
					log.Printf("Unable to init snap during process pod for node %s: %s", nodeName, err.Error())
				}
				node.Run(s.isOutsideCluster)
			}
		}
		return
	}

	// other event put into channel
	if s.isServicePod(e.Cur) {
		node.PodEvents <- e
	}

}

func (s *SnapTaskController) ProcessDeployment(e *DeploymentEvent) {
	switch e.EventType {
	case ADD:
	case DELETE:
	case UPDATE:
	}
}

func (s *SnapTaskController) ProcessDaemonSet(e *DaemonSetEvent) {
	switch e.EventType {
	case ADD:
	case DELETE:
	case UPDATE:
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
