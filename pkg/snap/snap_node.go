package snap

import (
	"log"
	"sync"

	"encoding/json"
	"fmt"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"strconv"
	"strings"
)

type CreatedByAnnotation struct {
	Kind       string
	ApiVersion string
	Reference  struct {
		Kind            string
		Namespace       string
		Name            string
		Uid             string
		ApiVersion      string
		ResourceVersion string
	}
}

type ServicePodInfo struct {
	Namespace          string
	Port               int32
	ParentResourceKind string
	ParentResourceName string
	Service            string
	PodName            string
}

func (servicePod *ServicePodInfo) buildPrometheusMetricURL() (string, error) {
	switch servicePod.ParentResourceKind {
	case "StatefulSet":
		return fmt.Sprintf("http://%s.%s.%s.svc.cluster.local:%s",
			servicePod.PodName, servicePod.ParentResourceName, servicePod.Namespace,
			strconv.Itoa(int(servicePod.Port))), nil
	case "ReplicaSet":
		// todo: better way to match deployment
		return fmt.Sprintf("http://%s.%s:%s",
			servicePod.PodName, servicePod.Namespace, strconv.Itoa(int(servicePod.Port))), nil
	default:
		return "", errors.New(fmt.Sprintf("[ ServicePodInfo ] Can't build Prometheus Metric URL, It's not supported Kind type {%s}", servicePod.ParentResourceKind))
	}
}

func NewServicePodInfo(pod *v1.Pod, k8sClient *kubernetes.Clientset) (*ServicePodInfo, error) {

	var createdMeta CreatedByAnnotation
	err := json.Unmarshal([]byte(pod.ObjectMeta.Annotations["kubernetes.io/created-by"]), &createdMeta)
	if err != nil {
		log.Printf("[ ServicePodInfo ] Can't unmarshal Pod annotation")
		return nil, err
	}

	services, err := common.ListServices(k8sClient, pod.Namespace)
	if err != nil {
		log.Printf("[ ServicePodInfo ] Can't get all service in namespace {%s}: %s", pod.Namespace, err.Error())
		return nil, err
	}

	container := pod.Spec.Containers[0]
	podInfo := &ServicePodInfo{
		Namespace:          pod.Namespace,
		Port:               container.Ports[0].HostPort,
		ParentResourceKind: createdMeta.Reference.Kind,
		ParentResourceName: createdMeta.Reference.Name,
		PodName:            pod.Name,
	}

	// todo: multiple service match one pod ?
	for _, service := range services {
		serviceSelector := labels.Set(service.Spec.Selector).AsSelector()
		// todo: check if kubernetes Service match any Pod
		if serviceSelector.Matches(labels.Set(pod.Labels)) && service.Name != "kubernetes" {
			podInfo.Service = service.Name
			log.Printf("[ ServicePodInfo ] Found Service=%s for %s", service.Name, pod.Name)
			return podInfo, nil
		}
	}

	return nil, errors.New(fmt.Sprintf(
		"[ ServicePodInfo ] Can't find Service for %s {%s} in namespace {%s}",
		createdMeta.Reference.Kind, createdMeta.Reference.Name, pod.Namespace))
}

type RunningServiceList struct {
	Lock               *sync.Mutex
	RunningServicePods map[string]*ServicePodInfo
	SnapNode           *SnapNode
}

func NewRunningServiceList(node *SnapNode) *RunningServiceList {
	return &RunningServiceList{
		Lock:               &sync.Mutex{},
		RunningServicePods: make(map[string]*ServicePodInfo),
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

func (runningService *RunningServiceList) addPodInfo(name string, podInfo *ServicePodInfo) {
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
			log.Printf("[ RunningServiceList ] Delete Running Service Pod {%s} in Node {%s} ", p.Name, runningService.SnapNode.NodeId)
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

func (snapTask *SnapTaskList) createTask(servicePodName string, podInfo *ServicePodInfo) error {
	snapTask.Lock.Lock()
	defer snapTask.Lock.Unlock()

	n := snapTask.SnapNode
	_, ok := snapTask.SnapTasks[servicePodName]
	if !ok {
		task, err := NewPrometheusCollectorTask(podInfo, n.config)
		if err != nil {
			log.Printf("[ SnapTaskList ] Build task fail: %s", err.Error())
			return err
		}
		_, err = n.TaskManager.CreateTask(task, n.config)
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
	ServiceList        *ServiceWatchingList
	config             *viper.Viper
	K8sClient          *kubernetes.Clientset
}

func NewSnapNode(nodeName string, externalIp string, serviceList *ServiceWatchingList, config *viper.Viper, kclient *kubernetes.Clientset) *SnapNode {
	snapNode := &SnapNode{
		NodeId:      nodeName,
		ExternalIP:  externalIp,
		TaskManager: nil,
		PodEvents:   make(chan *common.PodEvent, 1000),
		ExitChan:    make(chan bool, 1),
		ServiceList: serviceList,
		config:      config,
		K8sClient:   kclient,
	}
	runningServices := NewRunningServiceList(snapNode)
	snapTasks := NewSnapTaskList(snapNode)
	snapNode.RunningServicePods = runningServices
	snapNode.SnapTasks = snapTasks
	return snapNode
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
		if n.ServiceList.isServicePod(p) {
			// TODO: How do we know which container has the right port? and which port?
			container := p.Spec.Containers[0]
			if len(container.Ports) > 0 {
				podInfo, err := NewServicePodInfo(p, n.K8sClient)
				if err != nil {
					log.Printf("[ SnapNode ] Failed to create ServicePodInfo when Init SnapNode: %s", err.Error())
					return err
				}
				n.RunningServicePods.addPodInfo(p.Name, podInfo)
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
					podInfo, err := NewServicePodInfo(e.Cur, n.K8sClient)
					if err != nil {
						log.Printf("[ SnapNode ] Failed to create ServicePodInfo when Process event: %s", err.Error())
						continue
					}
					n.RunningServicePods.addPodInfo(e.Cur.Name, podInfo)
					if err := n.reconcileSnapState(); err != nil {
						log.Printf("[ SnapNode ] Unable to reconcile snap state: %s", err.Error())
						continue
					}
					log.Printf("[ SnapNode ] Insert Service Pod {%s}", e.Cur.Name)

				case common.DELETE:
					n.RunningServicePods.delPodInfo(e.Cur.Name)
					if err := n.reconcileSnapState(); err != nil {
						log.Printf("[ SnapNode ] Unable to reconcile snap state: %s", err.Error())
						continue
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
