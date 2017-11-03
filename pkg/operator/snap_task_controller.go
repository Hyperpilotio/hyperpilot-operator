package operator

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/snap"
)

type SnapNodeInfo struct {
	NodeId     string
	ExternalIP string
	SnapClient *snap.TaskManager

	// servicePodName <-> Task name
	SnapTasks map[string]string

	// servicePodName <-> nul
	RunningServicePods map[string]struct{}
}

type SnapTaskController struct {
	isOutsideCluster bool
	ServiceList      []string
	Nodes            map[string]*SnapNodeInfo
}

func NewSnapTaskController(runOutsideCluster bool) *SnapTaskController {
	return &SnapTaskController{
		ServiceList: []string{
			//There will be other type in the future
			"resource-worker",
		},
		Nodes:            map[string]*SnapNodeInfo{},
		isOutsideCluster: runOutsideCluster,
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
// 		Create a new snap task for that new pod
//		return true if reconcile Snap OK
// If any tasks in SnapTasks points to a pod not in the running service pods list,
// 		Delete the task from that snap.
//		return true if reconcile Snap OK
//}
//todo: snap is not ready yet
func (s *SnapTaskController) reconcileSnapState() bool {

	// Check each node
	for _, node := range s.Nodes {
		//If any pod in runningServicePods doesn't exist in SnapTasks value,
		// Create a new snap task for that new pod
		for servicePodName := range node.RunningServicePods {
			_, ok := node.SnapTasks[servicePodName]

			if !ok {
				task := snap.NewTask(servicePodName)
				_, err := node.SnapClient.CreateTask(task)
				if err != nil {
					log.Printf(err.Error())
					return false
				}
				node.SnapTasks[servicePodName] = task.Name
				return true
			}
		}

		// If any tasks in SnapTasks points to a pod not in the running service pods list,
		// Delete the task from that snap.
		for servicePodName, taskName := range node.SnapTasks {
			_, ok := node.RunningServicePods[servicePodName]
			if !ok {
				taskid, err := s.getTaskIDFromTaskName(node.SnapClient, taskName)
				if err != nil {
					log.Printf(err.Error())
					return false
				}
				node.SnapClient.StopTask(taskid)
				node.SnapClient.RemoveTask(taskid)
				delete(node.SnapTasks, servicePodName)
			}
		}
	}
	return true
}

func (s *SnapTaskController) getTaskIDFromTaskName(manager *snap.TaskManager, name string) (string, error) {
	tasks := manager.GetTasks().ScheduledTasks
	for _, t := range tasks {
		if t.Name == name {
			return t.ID, nil
		}
	}
	return "", errors.New("Can not find ID from Name")
}

func (s *SnapTaskController) Init(opertor *HyperpilotOperator) error {
	// Initialize steps:
	// 1. Create SnapNodeInfo for each node, TaskManager default is nil
	for _, n := range opertor.nodes {
		s.Nodes[n.NodeName] = &SnapNodeInfo{
			NodeId:             n.NodeName,
			ExternalIP:         n.ExternalIP,
			RunningServicePods: make(map[string]struct{}),
			SnapTasks:          make(map[string]string),
			SnapClient:         nil,
		}
	}

	// 2. If possible, find snap pod for each node
	//     if snap exist, create TaskManager
	//     if snap no exist, leave it nil
	for _, p := range opertor.pods {
		if strings.HasPrefix(p.PodName, "snap-") {
			if s.Nodes[p.NodeName].SnapClient == nil {
				var taskmanager *snap.TaskManager
				var err error
				if s.isOutsideCluster {
					// If run outside cluster, task manager communicate with snap with snap HOST IP
					taskmanager, err = snap.NewTaskManager(s.Nodes[p.NodeId].ExternalIP)
				} else {
					// if inside k8s cluster, task manager communicate with snap with snap POD ip
					taskmanager, err = snap.NewTaskManager(p.PodIP)
				}

				if err != nil {
					log.Printf("Failed to create Snap Task Manager: " + err.Error())
				}
				s.Nodes[p.NodeName].SnapClient = taskmanager
			}
		}
	}

	// 3. Get all running tasks from Snap API for existing snap pod
	for _, node := range s.Nodes {
		if node.SnapClient != nil {
			tasks := node.SnapClient.GetTasks()
			for _, task := range tasks.ScheduledTasks {
				// only look for task which controller care about
				if s.isSnapControllerTask(task.Name) {
					servicePodName := s.getServicePodNameFromSnapTask(task.Name)
					node.SnapTasks[servicePodName] = task.Name
				}
			}
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

	s.printOut()
	return nil
}

// Task name format PROMETHEUS-<POD_NAME>
// e.g. PROMETHEUS-resource-worker-spark-9br5d
func (s *SnapTaskController) getServicePodNameFromSnapTask(taskname string) string {
	return strings.SplitN(taskname, "-", 2)[1]
}

//we only want task name start with PROMETHEUS_TASK_NAME_PREFIX
func (s *SnapTaskController) isSnapControllerTask(taskname string) bool {
	if strings.HasPrefix(taskname, snap.PROMETHEUS_TASK_NAME_PREFIX) {
		return true
	}
	return false
}

func (s *SnapTaskController) ProcessNode(e *NodeEvent) {
	switch e.EventType {
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

	switch e.EventType {
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
					if s.reconcileSnapState() == false {
						delete(nodeInfo.RunningServicePods, e.Cur.Name)
						return
					}
					log.Printf("[ Controller ] Insert Pod {%s}", e.Cur.Name)
					return
				}
			}

			// If this is a snap pod, we need to update the snap client to point to the new Snap
			if strings.HasPrefix(e.Cur.Name, "snap-") {
				var taskmanager *snap.TaskManager
				var err error
				if s.isOutsideCluster {
					// If run outside cluster, task manager communicate with snap with snap HOST IP
					taskmanager, err = snap.NewTaskManager(s.Nodes[e.Cur.Spec.NodeName].ExternalIP)
				} else {
					// if inside k8s cluster, task manager communicate with snap with snap POD ip
					taskmanager, err = snap.NewTaskManager(e.Cur.Status.PodIP)
				}

				if err != nil {
					log.Printf("Failed to create Snap Task Manager: " + err.Error())
				}

				if nodeInfo.SnapClient != nil {
					// snap is re-created
					nodeInfo.SnapClient = taskmanager
					nodeInfo.SnapTasks = map[string]string{}
					log.Printf("[ Controller ] Found Snap Pod {%s}", e.Cur.Name)
					s.reconcileSnapState()
					return
				} else {
					// Snap not running when controller start,
					// Do Initialize Step 3 when snap pod start
					nodeInfo.SnapClient = taskmanager
					tasks := nodeInfo.SnapClient.GetTasks()

					for _, task := range tasks.ScheduledTasks {
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
