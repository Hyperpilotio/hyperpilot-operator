package snap

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/operator"
	"github.com/spf13/viper"
	"gopkg.in/resty.v1"
	"k8s.io/client-go/pkg/api/v1"
)

type SnapTaskController struct {
	ServiceList    []string
	snapNodeMx     *sync.Mutex
	Nodes          map[string]*SnapNode
	ClusterState   *common.ClusterState
	config         *viper.Viper
	ApplicationSet *common.StringSet
}

func NewSnapTaskController(config *viper.Viper) *SnapTaskController {
	return &SnapTaskController{
		ServiceList:    config.GetStringSlice("SnapTaskController.ServiceList"),
		snapNodeMx:     &sync.Mutex{},
		Nodes:          make(map[string]*SnapNode),
		config:         config,
		ApplicationSet: common.NewStringSet(),
	}
}

func (s *SnapTaskController) GetResourceEnum() operator.ResourceEnum {
	return operator.POD
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

func (s *SnapTaskController) Init(clusterState *common.ClusterState) error {
	s.ClusterState = clusterState

	s.ClusterState.Lock.RLock()
	for _, n := range clusterState.Nodes {
		for _, p := range clusterState.Pods {
			if p.Spec.NodeName == n.NodeName && isSnapPod(p) {
				snapNode := NewSnapNode(n.NodeName, n.ExternalIP, &s.ServiceList, s.config)
				init, err := snapNode.init(s.isOutsideCluster(), clusterState)
				if !init {
					log.Printf("[ SnapTaskController ] Snap is not found in the cluster for node during init: %s", n.NodeName)
					// We will assume a new snap will be running and we will be notified at ProcessPod
				} else if err != nil {
					log.Printf("[ SnapTaskController ] Unable to init snap for node %s: %s", n.NodeName, err.Error())
				} else {
					s.snapNodeMx.Lock()
					s.Nodes[n.NodeName] = snapNode
					s.snapNodeMx.Unlock()
				}
			}
		}
	}
	s.ClusterState.Lock.RUnlock()

	log.Print("[ SnapTaskController ] Init() Finished.")
	if s.config.GetBool("SnapTaskController.Analyzer.Enable") {
		log.Printf("[ SnapTaskController ] Poll Analyzer is enabled")
		go s.pollingAnalyzer()
	}
	return nil
}

func (s *SnapTaskController) ProcessPod(e *common.PodEvent) {
	s.snapNodeMx.Lock()
	defer s.snapNodeMx.Unlock()

	switch e.EventType {
	case common.DELETE:
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
	case common.ADD, common.UPDATE:
		if e.Cur.Status.Phase == "Running" && (e.Old == nil || e.Old.Status.Phase == "Pending") {
			nodeName := e.Cur.Spec.NodeName
			node, ok := s.Nodes[nodeName]
			if !ok {
				if isSnapPod(e.Cur) {
					newNode := NewSnapNode(nodeName, s.ClusterState.Nodes[nodeName].ExternalIP, &s.ServiceList, s.config)
					log.Printf("[ SnapTaskController ] Create new SnapNode in {%s}.", newNode.NodeId)
					s.Nodes[nodeName] = newNode
					go func() {
						if err := newNode.initSnap(s.isOutsideCluster(), e.Cur, s.ClusterState); err != nil {
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

func getServicePodNameFromSnapTask(taskName string) string {
	return strings.SplitN(taskName, "-", 2)[1]
}

func isSnapControllerTask(taskName string) bool {
	if strings.HasPrefix(taskName, prometheusTaskNamePrefix) {
		return true
	}
	return false
}

func (s *SnapTaskController) String() string {
	return fmt.Sprintf("SnapTaskController")
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
				":", s.config.GetInt("SnapTaskController.Analyzer.Port"), apiApps)
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
		":", s.config.GetInt("SnapTaskController.Analyzer.Port"), apiK8sServices)

	svcResp := ServiceResponse{}
	if !s.isSnapNodeReady() {
		log.Printf("SnapNodes are not ready")
		return
	}
	if !s.isAppSetChanged(appResps) {
		return
	}
	log.Printf("[ SnapTaskController ] Application Set change")

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
				s.updateServiceList(deployResponse.Data.Name)
				s.updateRunningServicePods(pods)
			case "StatefulSet":
				statefulResponse := K8sStatefulSetResponse{}
				err = json.Unmarshal(resp.Body(), &statefulResponse)
				if err != nil {
					log.Print("JSON parse error: " + err.Error())
					return
				}
				pods := s.ClusterState.FindStatefulSetPod(statefulResponse.Data.Namespace, statefulResponse.Data.Name)
				s.updateServiceList(statefulResponse.Data.Name)
				s.updateRunningServicePods(pods)
			default:
				log.Printf("Not supported service kind {%s}", svcResp.Kind)
			}
		}
	}
	s.reconcileSnapState()
}

func (s *SnapTaskController) updateRunningServicePods(pods []*v1.Pod) {
	//add
	for _, p := range pods {
		snapNode := s.Nodes[p.Spec.NodeName]
		container := p.Spec.Containers[0]
		if _, ok := snapNode.RunningServicePods[p.Name]; !ok {
			snapNode.runningPodsMx.Lock()
			snapNode.RunningServicePods[p.Name] = ServicePodInfo{
				Namespace: p.Namespace,
				Port:      container.Ports[0].HostPort,
			}
			snapNode.runningPodsMx.Unlock()
			log.Printf("add Running Service Pod {%s} in Node {%s}. ", p.Name, snapNode.NodeId)
		}
	}

	//del
	for _, snapNode := range s.Nodes {
		for podName := range snapNode.RunningServicePods {
			if !containPod(pods, podName) {
				snapNode.runningPodsMx.Lock()
				delete(snapNode.RunningServicePods, podName)
				snapNode.runningPodsMx.Unlock()
				log.Printf("delete Running Service Pod {%s} in Node {%s} ", podName, snapNode.NodeId)
			}
		}
	}
}

// todo: lock!
// todo: check overwrite by analyzer result
func (s *SnapTaskController) updateServiceList(deployName string) {
	s.ServiceList = append(s.ServiceList, deployName)
}

func containPod(pods []*v1.Pod, podName string) bool {
	for _, pod := range pods {
		if podName == pod.Name {
			return true
		}
	}
	return false
}

func (s *SnapTaskController) isAppSetChanged(appResps []AppResponse) bool {
	appSet := common.NewStringSet()
	for _, app := range appResps {
		appSet.Add(app.AppID)
	}

	if appSet.IsIdentical(s.ApplicationSet) {
		s.ApplicationSet = appSet
		return false
	} else {
		s.ApplicationSet = appSet
		return true
	}
}

func (s *SnapTaskController) isSnapNodeReady() bool {
	s.ClusterState.Lock.RLock()
	defer s.ClusterState.Lock.RUnlock()

	for nodeName := range s.ClusterState.Nodes {
		if s.Nodes[nodeName] == nil {
			return false
		}

		if s.Nodes[nodeName].TaskManager.isReady() == false {
			return false
		}
	}
	return true
}

func (s *SnapTaskController) isOutsideCluster() bool {
	return s.config.GetBool("Operator.OutsideCluster")
}

func (s *SnapTaskController) ProcessDeployment(e *common.DeploymentEvent) {}

func (s *SnapTaskController) ProcessDaemonSet(e *common.DaemonSetEvent) {}

func (s *SnapTaskController) ProcessNode(e *common.NodeEvent) {}

func (s *SnapTaskController) ProcessReplicaSet(e *common.ReplicaSetEvent) {}

func (s *SnapTaskController) Close() {}
