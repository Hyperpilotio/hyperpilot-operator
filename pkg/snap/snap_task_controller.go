package snap

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/operator"
	"github.com/spf13/viper"
	"gopkg.in/resty.v1"
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
	config             *viper.Viper
}

type SnapTaskController struct {
	isOutsideCluster bool
	ServiceList      []string
	Nodes            map[string]*SnapNode
	ClusterState     *common.ClusterState
	config           *viper.Viper
	ApplicationSet   *common.StringSet
}

func NewSnapTaskController(runOutsideCluster bool, config *viper.Viper) *SnapTaskController {
	return &SnapTaskController{
		ServiceList:      config.GetStringSlice("SnapTaskController.ServiceList"),
		Nodes:            make(map[string]*SnapNode),
		isOutsideCluster: runOutsideCluster,
		config:           config,
		ApplicationSet:   common.NewStringSet(),
	}
}

func (s *SnapTaskController) GetResourceEnum() operator.ResourceEnum {
	return operator.POD | operator.NODE
}

func (node *SnapNode) reconcileSnapState() error {
	for servicePodName, podInfo := range node.RunningServicePods {
		_, ok := node.SnapTasks[servicePodName]
		if !ok {
			task := NewPrometheusCollectorTask(servicePodName, podInfo.Namespace, podInfo.Port, node.config)
			_, err := node.TaskManager.CreateTask(task, node.config)
			if err != nil {
				return err
			}
			node.SnapTasks[servicePodName] = task.Name
			log.Printf("[ SnapNode ] Create task {%s} in Node {%s}", task.Name, node.NodeId)
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
			log.Printf("[ SnapNode ] Delete task {%s} in Node {%s}", taskName, node.NodeId)
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

func NewSnapNode(nodeName string, externalIp string, serviceList []string, config *viper.Viper) *SnapNode {
	return &SnapNode{
		NodeId:             nodeName,
		ExternalIP:         externalIp,
		RunningServicePods: make(map[string]ServicePodInfo),
		SnapTasks:          make(map[string]string),
		TaskManager:        nil,
		PodEvents:          make(chan *operator.PodEvent, 1000),
		ExitChan:           make(chan bool, 1),
		ServiceList:        serviceList,
		config:             config,
	}
}

func (s *SnapTaskController) Init(clusterState *common.ClusterState) error {
	s.ClusterState = clusterState
	for _, n := range clusterState.Nodes {
		for _, p := range clusterState.Pods {
			if p.Spec.NodeName == n.NodeName && isSnapPod(p) {
				snapNode := NewSnapNode(n.NodeName, n.ExternalIP, s.ServiceList, s.config)
				init, err := snapNode.init(s.isOutsideCluster, clusterState)
				if !init {
					log.Printf("[ SnapTaskController ] Snap is not found in the cluster for node during init: %s", n.NodeName)
					// We will assume a new snap will be running and we will be notified at ProcessPod
				} else if err != nil {
					log.Printf("[ SnapTaskController ] Unable to init snap for node %s: %s", n.NodeName, err.Error())
				} else {
					s.Nodes[n.NodeName] = snapNode
				}
			}
		}
	}
	go s.pollingAnalyzer()
	return nil
}

func (n *SnapNode) init(isOutsideCluster bool, clusterState *common.ClusterState) (bool, error) {
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
			log.Printf("[ SnapTaskController ] Delete SnapNode in {%s}", node.NodeId)
			node.Exit()
			delete(s.Nodes, nodeName)
			return
		}

		if node.isServicePod(e.Cur) {
			node.PodEvents <- e
		}
	case operator.ADD, operator.UPDATE:
		if e.Cur.Status.Phase == "Running" && (e.Old == nil || e.Old.Status.Phase == "Pending") {
			nodeName := e.Cur.Spec.NodeName
			node, ok := s.Nodes[nodeName]
			if !ok {
				if isSnapPod(e.Cur) {
					newNode := NewSnapNode(nodeName, s.ClusterState.Nodes[nodeName].ExternalIP, s.ServiceList, s.config)
					log.Printf("[ SnapTaskController ] Create new SnapNode in {%s}.", newNode.NodeId)
					s.Nodes[nodeName] = newNode
					go func() {
						if err := newNode.initSnap(s.isOutsideCluster, e.Cur, s.ClusterState); err != nil {
							// todo: fix crash because current map write
							//delete(s.Nodes, nodeName)
							log.Printf("[ SnapTaskController ] Unable to init snap for node %s: %s", nodeName, err.Error())
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
				log.Printf("[ SnapNode ] {%s} signaled to exit", n.NodeId)
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
					log.Printf("[ SnapNode ] Insert Service Pod {%s}", e.Cur.Name)

				case operator.DELETE:
					if _, ok := n.RunningServicePods[e.Cur.Name]; ok {
						delete(n.RunningServicePods, e.Cur.Name)
						if err := n.reconcileSnapState(); err != nil {
							log.Printf("Unable to reconcile snap state: %s", err.Error())
							return
						}
						log.Printf("[ SnapNode ] Delete Service Pod {%s}", e.Cur.Name)
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

func (s *SnapTaskController) pollingAnalyzer() {
	tick := time.Tick(3 * time.Second)
	for {
		select {
		case <-tick:
			analyzerURL := fmt.Sprintf("%s%s%s%d%s",
				"http://", s.config.GetString("SnapTaskController.Analyzer.Address"),
				":", s.config.GetInt("SnapTaskController.Analyzer.Port"), API_APPS)
			appResp := AppResponses{}
			resp, err := resty.R().Get(analyzerURL)
			if err != nil {
				log.Print("http GET error: " + err.Error())
				return
			}
			err = json.Unmarshal(resp.Body(), &appResp)
			if err != nil {
				log.Print("JSON parse error: " + err.Error())
				return
			}
			s.checkApplications(appResp.Data)
		}
	}
}

func (s *SnapTaskController) checkApplications(appResps []AppResponse) {
	analyzerURL := fmt.Sprintf("%s%s%s%d%s",
		"http://", s.config.GetString("SnapTaskController.Analyzer.Address"),
		":", s.config.GetInt("SnapTaskController.Analyzer.Port"), API_K8SSERVICES)

	svcResp := ServiceResponse{}
	if !s.isSnapNodeReady() {
		log.Printf("SnapNodes are not ready")
		return
	}
	if !s.isAppSetChange(appResps) {
		return
	}
	log.Printf("Application Set change")

	// app set is empty
	if len(appResps) == 0 {
		pods := []*v1.Pod{}
		s.updateRunningServicePods(pods)
	}
	// app set not empty
	for _, app := range appResps {
		for _, svc := range app.Microservices {
			serviceURL := analyzerURL + "/" + svc.ServiceID
			resp, err := resty.R().Get(serviceURL)
			if err != nil {
				log.Print("http GET error: " + err.Error())
				return
			}

			err = json.Unmarshal(resp.Body(), &svcResp)
			if err != nil {
				log.Print("JSON parse error: " + err.Error())
				return
			}

			switch svcResp.Kind {
			case "Deployment":
				deployResponse := K8sDeploymentResponse{}
				err = json.Unmarshal(resp.Body(), &deployResponse)
				if err != nil {
					log.Print("JSON parse error: " + err.Error())
					return
				}
				hash, err := s.ClusterState.FindReplicaSetHash(deployResponse.Data.Name)
				if err != nil {
					log.Printf("[ SnapTaskController ] pod-template-hash is not found for deployment {%s}", deployResponse.Data.Name)
					continue
				}
				pods := s.ClusterState.FindDeploymentPod(deployResponse.Data.Namespace, deployResponse.Data.Name, hash)
				s.updateRunningServicePods(pods)
			case "StatefulSet":
				statefulResponse := K8sStatefulSetResponse{}
				err = json.Unmarshal(resp.Body(), &statefulResponse)
				if err != nil {
					log.Print("JSON parse error: " + err.Error())
					return
				}
				pods := s.ClusterState.FindStatefulSetPod(statefulResponse.Data.Namespace, statefulResponse.Data.Name)
				s.updateRunningServicePods(pods)

			default:
				log.Printf("Not supported service kind {%s}", svcResp.Kind)
			}
		}
	}
	s.reconcileSnapState()
}

//todo: update SnapTaskController & SnapNode ServiceList ??
func (s *SnapTaskController) updateRunningServicePods(pods []*v1.Pod) {
	//add
	for _, p := range pods {
		snapNode := s.Nodes[p.Spec.NodeName]
		container := p.Spec.Containers[0]
		if _, ok := snapNode.RunningServicePods[p.Name]; !ok {
			snapNode.RunningServicePods[p.Name] = ServicePodInfo{
				Namespace: p.Namespace,
				Port:      container.Ports[0].HostPort,
			}
			log.Printf("add Running Service Pod {%s} in Node {%s}. ", p.Name, snapNode.NodeId)
		}
	}

	//del
	for _, snapNode := range s.Nodes {
		for podName := range snapNode.RunningServicePods {
			if !isExist(pods, podName) {
				delete(snapNode.RunningServicePods, podName)
				log.Printf("delete Running Service Pod {%s} in Node {%s} ", podName, snapNode.NodeId)
			}
		}
	}
}

func isExist(pods []*v1.Pod, podName string) bool {
	for _, pod := range pods {
		if podName == pod.Name {
			return true
		}
	}
	return false
}

// true : change
// false : no change
func (s *SnapTaskController) isAppSetChange(appResps []AppResponse) bool {
	app_set := common.NewStringSet()
	for _, app := range appResps {
		app_set.Add(app.AppID)
	}

	if app_set.IsIdentical(s.ApplicationSet) {
		s.ApplicationSet = app_set
		return false
	} else {
		s.ApplicationSet = app_set
		return true
	}
}

// true : ready
// false : not ready
func (s *SnapTaskController) isSnapNodeReady() bool {
	for nodeName := range s.ClusterState.Nodes {
		if s.Nodes[nodeName] == nil {
			return false
		}
	}
	return true
}

func (s *SnapTaskController) ProcessDeployment(e *operator.DeploymentEvent) {}

func (s *SnapTaskController) ProcessDaemonSet(e *operator.DaemonSetEvent) {}

func (s *SnapTaskController) ProcessNode(e *operator.NodeEvent) {}

func (s *SnapTaskController) ProcessReplicaSet(e *operator.ReplicaSetEvent) {}
