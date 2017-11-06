package operator

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/snap"
	"sync"
	"k8s.io/client-go/pkg/api/v1"
	"time"
)

type ServicePodInfo struct {
	Namespace string
	Port      int32
}

type SnapNodeInfo struct {
	NodeId     string
	ExternalIP string
	SnapClient *snap.TaskManager

	// servicePodName <-> Task name
	SnapTasks map[string]string

	// servicePodName <-> pod info
	RunningServicePods map[string]ServicePodInfo

	wg *sync.WaitGroup
}

type SnapTaskController struct {
	isOutsideCluster bool
	ServiceList      []string
	Nodes            map[string]*SnapNodeInfo
	EventBuf		 chan *PodEvent
	snapWg			 *sync.WaitGroup
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
func (s *SnapTaskController) reconcileSnapState() bool {
	// Check each node
	for _, node := range s.Nodes {
		// If any pod in runningServicePods doesn't exist in SnapTasks value,
		// Create a new snap task for that new pod
		for servicePodName, podInfo := range node.RunningServicePods {
			_, ok := node.SnapTasks[servicePodName]
			if !ok {
				task := snap.NewPrometheusCollectorTask(servicePodName, podInfo.Namespace, podInfo.Port)
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

func (s *SnapTaskController) Init(operator *HyperpilotOperator) error {
	// Initialize steps:
	// 1. Create SnapNodeInfo for each node, TaskManager default is nil
	for _, n := range operator.nodes {
		s.Nodes[n.NodeName] = &SnapNodeInfo{
			NodeId:             n.NodeName,
			ExternalIP:         n.ExternalIP,
			RunningServicePods: make(map[string]ServicePodInfo),
			SnapTasks:          make(map[string]string),
			SnapClient:         nil,
		}
	}


	// 2. Get all running service pods
	for _, service := range s.ServiceList {
		for _, p := range operator.pods {
			if strings.HasPrefix(p.Name, service) {
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
	}

	// 3. create snap pod for each node, and Get all running tasks from Snap API for existing snap pod
	for _, n := range s.Nodes {
		s.snapWg.Add(1)
		go func() {
			s.StartSnapAtInit(n, operator)
		}()
	}

	go func(){
		s.porcessPod()
	}()
	s.printOut()

	return nil

	// 2. If possible, find snap pod for each node
	//     if snap exist, create TaskManager
	//     if snap no exist, leave it nil


	//for _, p := range operator.pods {
	//	if strings.HasPrefix(p.Name, "snap-") {
			//if s.Nodes[p.Spec.NodeName].SnapClient == nil {
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
			//	s.Nodes[p.Spec.NodeName].SnapClient = taskmanager
			//}
	//	}
	//}


	// 3. Get all running tasks from Snap API for existing snap pod
	//for _, node := range s.Nodes {
	//	if node.SnapClient != nil {
	//		tasks := node.SnapClient.GetTasks()
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

func (s *SnapTaskController) StartSnapAtInit(n *SnapNodeInfo, operator *HyperpilotOperator){

	var taskmanager *snap.TaskManager
	var err error

	if n.SnapClient == nil {
		if s.isOutsideCluster {
			// If run outside cluster, task manager communicate with snap with snap HOST IP
			taskmanager, err = snap.NewTaskManager(n.ExternalIP)
		} else {
			// if inside k8s cluster, task manager communicate with snap with snap POD ip
			var podIP string
			for _,p := range operator.pods{
				if n.NodeId == p.Spec.NodeName && strings.HasPrefix(p.Name, "snap-"){
					podIP = p.Status.PodIP
				}
			}
			taskmanager, err = snap.NewTaskManager(podIP)
		}

		if err != nil {
			log.Printf("Failed to create Snap Task Manager: " + err.Error())
		}
		n.SnapClient = taskmanager
	}

	for taskmanager.IsReady(){
		//todo: timeout
		log.Println("wait for Snap Task Manager Load plugin complete")
		time.Sleep(time.Second * 10)
	}

	// Get all running tasks from Snap API for existing snap pod
	tasks := taskmanager.GetTasks()
	for _, task := range tasks.ScheduledTasks {
		// only look for task which controller care about
		if s.isSnapControllerTask(task.Name) {
			servicePodName := s.getServicePodNameFromSnapTask(task.Name)
			n.SnapTasks[servicePodName] = task.Name
		}
	}
	s.snapWg.Done()
}

func(s *SnapTaskController) StartSnapAtEvent(p *v1.Pod){

	nodeInfo := s.Nodes[p.Spec.NodeName]
	if nodeInfo.SnapClient != nil {
		// snap is re-created
		s.snapWg.Add(1)
	}

	var taskmanager *snap.TaskManager
	var err error
	if s.isOutsideCluster {
		// If run outside cluster, task manager communicate with snap with snap HOST IP
		taskmanager, err = snap.NewTaskManager(s.Nodes[p.Spec.NodeName].ExternalIP)
	} else {
		// if inside k8s cluster, task manager communicate with snap with snap POD ip
		taskmanager, err = snap.NewTaskManager(p.Status.PodIP)
	}

	if err != nil {
		log.Printf("Failed to create Snap Task Manager: " + err.Error())
	}

	for taskmanager.IsReady(){
		//todo: timeout
		log.Println("wait for Snap Task Manager Load plugin complete")
		time.Sleep(time.Second * 10)
	}


	if nodeInfo.SnapClient != nil {
		// snap is re-created
		nodeInfo.SnapClient = taskmanager
		nodeInfo.SnapTasks = map[string]string{}
		log.Printf("[ Controller ] Found Snap Pod {%s}", p.Name)
		s.snapWg.Done()
		s.reconcileSnapState()
		return
	} else {
		// Snap not running when controller start, Initialize Step 3 when snap pod start
		nodeInfo.SnapClient = taskmanager
		tasks := nodeInfo.SnapClient.GetTasks()
		for _, task := range tasks.ScheduledTasks {
			servicePodName := s.getServicePodNameFromSnapTask(task.Name)
			nodeInfo.SnapTasks[servicePodName] = task.Name
		}
		s.snapWg.Done() // Add() in Init()
		return
	}

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
	// process snap separately
	// If this is a snap pod, we need to update the snap client to point to the new Snap

	if strings.HasPrefix(e.Cur.Name, "snap-"){
		switch e.EventType {
		case UPDATE:
			if e.Old.Status.Phase == "Pending" && e.Cur.Status.Phase == "Running" {
				s.StartSnapAtEvent(e.Cur)
			}
		}
		return
	}

	// other event put into channel
	s.EventBuf <- e
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

func (s *SnapTaskController) porcessPod(){
	for e := range s.EventBuf {
		s.snapWg.Wait()
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
						container := e.Cur.Spec.Containers[0]
						nodeInfo.RunningServicePods[e.Cur.Name] = ServicePodInfo{
							Namespace: e.Cur.Namespace,
							Port:      container.Ports[0].HostPort,
						}
						if s.reconcileSnapState() == false {
							delete(nodeInfo.RunningServicePods, e.Cur.Name)
							return
						}
						log.Printf("[ Controller ] Insert Pod {%s}", e.Cur.Name)
						return
					}
				}
			}
		}
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
