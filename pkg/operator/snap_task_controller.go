package operator

import (
	"fmt"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/snap"
	"log"
	"strings"
)

type SnapNodeInfo struct {
	NodeId     string
	SnapClient *snap.SnapClient

	// servicePodName <-> Task name
	SnapTasks map[string]string

	// servicePodName <-> nul
	RunningServicePods map[string]struct{}
}

type SnapTaskController struct {
	TaskController

	ServiceList []string
	Nodes       map[string]*SnapNodeInfo
}

func NewSnapTaskController() BaseController {
	return &SnapTaskController{
		ServiceList: []string{
			"resource-worker",
		},
		Nodes: map[string]*SnapNodeInfo{},
	}
}

func (s *SnapTaskController) GetResourceEnum() ResourceEnum {
	//return POD | NODE | DAEMONSET
	return POD | NODE
}

func (s *SnapTaskController) Close() {

}

//for each node {
// Compare node.SnapTasks and RunningServicePods
// If any pod in runningServicePods doesn't exist in SnapTasks value,
// Create a new snap task for that new pod
// If any tasks in SnapTasks points to a pod not in the running service pods list,
// Delete the task from that snap.
//}
func (s *SnapTaskController) reconcileSnapState() {

	// Check each node
	for _, node := range s.Nodes {
		//If any pod in runningServicePods doesn't exist in SnapTasks value,
		// Create a new snap task for that new pod
		for servicePodName := range node.RunningServicePods {
			_, ok := node.SnapTasks[servicePodName]

			if !ok {
				task := node.SnapClient.CreateSnapTask()
				node.SnapClient.RunTask(task)
				node.SnapTasks[servicePodName] = task.Name
			}
		}

		// If any tasks in SnapTasks points to a pod not in the running service pods list,
		// Delete the task from that snap.
		for servicePodName, taskName := range node.SnapTasks {
			_, ok := node.RunningServicePods[servicePodName]
			if !ok {
				node.SnapClient.DeleteTask(taskName)
				delete(node.SnapTasks, servicePodName)
			}
		}
	}
}

func (s *SnapTaskController) Init(opertor *HyperpilotOpertor) error {

	/* 1. Get all snap pods for each node, and create SnapClient for each snap
	//for _, p := range opertor.pods {
	//	if strings.HasPrefix(p.PodName, "snap-"){
	//		s.Nodes[p.NodeId] = SnapNodeInfo{
	//			NodeId: p.NodeId,
	//			RunningServicePods: make(map[string]struct{}),
	//			SnapClient: snap.NewSnapClient(p.PodName),
	//			SnapTasks: make(map[string]string),
	//		}
	//	}
	//}
	*/

	// Initialize steps:
	// 1. Create SnapNodeInfo for each node, SnapClient default is nil
	for _, n := range opertor.nodes {
		s.Nodes[n.NodeName] = &SnapNodeInfo{
			NodeId:             n.NodeName,
			RunningServicePods: make(map[string]struct{}),
			SnapTasks:          make(map[string]string),
			SnapClient:         nil,
		}
	}

	// 2. If possible, find snap pod for each node
	//     if snap exist, create SnapClient
	//     if snap no exist, leave it nil
	for _, p := range opertor.pods {
		if strings.HasPrefix(p.PodName, "snap-") {
			if s.Nodes[p.NodeName].SnapClient == nil {
				s.Nodes[p.NodeName].SnapClient = snap.NewSnapClient(p.PodName)
			}
		}
	}

	// 3. Get all running tasks from Snap API for existing snap pod
	for _, node := range s.Nodes {
		tasks, err := node.SnapClient.GetAllTasks()

		if err != nil {
			// todo: Check error
		}
		for _, task := range tasks {
			// Naming snap tasks bsased on service pod name, e.g: s1-task
			servicePodName := s.getServicePodNameFromSnapTask(task.Name)
			node.SnapTasks[servicePodName] = task.Name
		}
	}

	// 4. Get all running service pods
	for _, service := range s.ServiceList {
		for _, p := range opertor.pods {
			if strings.HasPrefix(p.PodName, service) {
				s.Nodes[p.NodeId].RunningServicePods[p.PodName] = struct{}{}
			}
		}
	}

	return nil
}

func (s *SnapTaskController) getServicePodNameFromSnapTask(taskname string) string {
	return strings.Split(taskname, "-")[0]
}

func (s *SnapTaskController) Receive(e Event) {

	_, ok := e.(*PodEvent)
	if ok {
		s.ProcessPod(e.(*PodEvent))
	}

	_, ok = e.(*DeploymentEvent)
	if ok {
		s.ProcessDeployment(e.(*DeploymentEvent))
	}

	_, ok = e.(*DaemonSetEvent)
	if ok {
		s.ProcessDaemonSet(e.(*DaemonSetEvent))
	}

	_, ok = e.(*NodeEvent)
	if ok {
		s.ProcessNode(e.(*NodeEvent))
	}

}

func (s *SnapTaskController) ProcessNode(e *NodeEvent) {
	switch e.Event_type {
	case ADD:
	case DELETE:
	case UPDATE:
	}
}

func (s *SnapTaskController) String() string {
	return fmt.Sprintf("SnapTaskController")
}

func (s *SnapTaskController) ProcessPod(e *PodEvent) {
	nodeInfo := s.Nodes[e.Cur.Spec.NodeName]

	switch e.Event_type {
	case ADD:
	case DELETE:
		// delete pod from nodeInfo.RunningServicePods
		for _, service := range s.ServiceList {
			if strings.HasPrefix(e.Cur.Name, service) {
				delete(nodeInfo.RunningServicePods, e.Cur.Name)
				s.reconcileSnapState()
				log.Printf("[ Controller ] Delete Pod {%s}", e.Cur.Name)
			}
		}

	case UPDATE:
		// Only check scheduled pods
		if e.Old.Status.Phase == "Pending" && e.Cur.Status.Phase == "Running" {
			// Check if it's running and is part of a service we're looking for
			for _, service := range s.ServiceList {
				if strings.HasPrefix(e.Cur.Name, service) {
					nodeInfo.RunningServicePods[e.Cur.Name] = struct{}{}
					s.reconcileSnapState()
					log.Printf("[ Controller ] Insert Pod {%s}", e.Cur.Name)
					return
				}
			}

			// If this is a snap pod, we need to update the snap client to point to the new Snap
			if strings.HasPrefix(e.Cur.Name, "snap-") {

				if nodeInfo.SnapClient != nil {
					// snap is re-created
					nodeInfo.SnapClient = snap.NewSnapClient(e.Cur.Name)
					nodeInfo.SnapTasks = map[string]string{}
					log.Printf("[ Controller ] Found Snap Pod {%s}", e.Cur.Name)
					s.reconcileSnapState()
					return
				} else {
					// Snap not running when controller start,
					// Do Initialize Step 3 when snap pod start
					nodeInfo.SnapClient = snap.NewSnapClient(e.Cur.Name)
					tasks, err := nodeInfo.SnapClient.GetAllTasks()

					if err != nil {
						// todo: Check error
					}
					for _, task := range tasks {
						servicePodName := s.getServicePodNameFromSnapTask(task.Name)
						nodeInfo.SnapTasks[servicePodName] = task.Name
					}

					return
				}
			}
		}
	}
}

func (s *SnapTaskController) ProcessDeployment(e *DeploymentEvent) {
	switch e.Event_type {
	case ADD:
	case DELETE:
	case UPDATE:
	}
}

func (s *SnapTaskController) ProcessDaemonSet(e *DaemonSetEvent) {
	switch e.Event_type {
	case ADD:
	case DELETE:
	case UPDATE:
	}
}

func (s *SnapTaskController) PrintOut() {

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
