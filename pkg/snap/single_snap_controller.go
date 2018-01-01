package snap

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/operator"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const hyperpilotSnapNamespace = "hyperpilot"
const hyperpilotSnapDeploymentName = "hyperpilot-snap"

type SingleSnapController struct {
	ServiceList      *ServiceWatchingList
	SnapNode         *SnapNode
	DeletingSnapNode *SnapNode
	config           *viper.Viper
	ClusterState     *common.ClusterState
	K8sClient        *kubernetes.Clientset
	analyzerPoller   *AnalyzerPoller
	ApplicationSet   *common.StringSet
}

type ServiceWatchingList struct {
	Lock         *sync.Mutex
	watchingList map[string][]string
}

func NewServiceWatchingList(defaultList []string) *ServiceWatchingList {
	list := ServiceWatchingList{
		Lock:         &sync.Mutex{},
		watchingList: make(map[string][]string),
	}
	list.watchingList["HP_DEFAULT"] = defaultList
	return &list
}

func (watchinglist *ServiceWatchingList) isServicePod(pod *v1.Pod) bool {
	watchinglist.Lock.Lock()
	defer watchinglist.Lock.Unlock()

	for _, serviceList := range watchinglist.watchingList {
		for _, service := range serviceList {
			if strings.HasPrefix(pod.Name, service) {
				return true
			}
		}
	}
	return false
}

func (watchinglist *ServiceWatchingList) add(appID, serviceName string) {
	watchinglist.Lock.Lock()
	defer watchinglist.Lock.Unlock()

	if _, ok := watchinglist.watchingList[appID]; !ok {
		watchinglist.watchingList[appID] = []string{}
	}
	watchinglist.watchingList[appID] = append(watchinglist.watchingList[appID], serviceName)
}

func (watchinglist *ServiceWatchingList) deleteWholeApp(appID string) {
	watchinglist.Lock.Lock()
	defer watchinglist.Lock.Unlock()
	delete(watchinglist.watchingList, appID)
}

func NewSingleSnapController(config *viper.Viper) *SingleSnapController {
	return &SingleSnapController{
		ServiceList:    NewServiceWatchingList(config.GetStringSlice("SnapTaskController.ServiceList")),
		SnapNode:       nil,
		config:         config,
		ApplicationSet: common.NewStringSet(),
	}
}

func (s *SingleSnapController) GetResourceEnum() operator.ResourceEnum {
	return operator.POD
}

func (s *SingleSnapController) Init(clusterState *common.ClusterState) error {
	s.ClusterState = clusterState
	// get k8s client
	s.ClusterState = clusterState
	kclient, err := common.NewK8sClient(s.isOutsideCluster())
	s.K8sClient = kclient
	if err != nil {
		log.Printf("[ SingleSnapController ] Create client failure")
		return err
	}
	deployClient := kclient.ExtensionsV1beta1Client.Deployments(hyperpilotSnapNamespace)

	// build snap spec
	deployment, err := common.CreateDeploymentFromYamlUrl(s.config.GetString("SnapTaskController.SnapDeploymentYamlURL"))
	if err != nil {
		log.Printf("[ SingleSnapController ] Canot read YAML file from url: %s ", err.Error())
		return err
	}
	deployment.Name = hyperpilotSnapDeploymentName
	deployment.Namespace = hyperpilotSnapNamespace

	// create snap deployment
	_, err = deployClient.Create(deployment)
	if err != nil {
		log.Printf("[ SingleSnapController ] Create Snap Deployment fail: %s ", err.Error())
		return err
	}

	// create SnapNode util deployment is available
	for {
		d, err := deployClient.Get(deployment.Name, metav1.GetOptions{})
		if err != nil {
			log.Printf("[ SingleSnapController ] Get deployment {%s} status fail: %s ", deployment.Name, err.Error())
			return err
		}

		var isAvailable v1.ConditionStatus
		for _, cond := range d.Status.Conditions {
			if cond.Type == v1beta1.DeploymentAvailable {
				isAvailable = cond.Status
			}
		}

		if isAvailable == v1.ConditionTrue {
			log.Printf("[ SingleSnapController ] Deployment {%s} is ready, create SnapNode", hyperpilotSnapDeploymentName)
			if err := s.createSnapNode(); err != nil {
				log.Printf("[ SingleSnapController ] Create SnapNode fail: %s ", err.Error())
				return err
			}
			log.Print("[ SingleSnapController ] SnapNode is creeated, Init() finished")
			break
		}
		log.Printf("[ SingleSnapController ] Wait for deployment {%s} become available", hyperpilotSnapDeploymentName)
		time.Sleep(5 * time.Second)
	}

	if s.config.GetBool("SnapTaskController.Analyzer.Enable") {
		log.Printf("[ SnapTaskController ] Poll Analyzer flag is enabled, launch goroutin to poll analyzer")
		s.analyzerPoller = NewAnalyzerPoller(s.config, s, 1*time.Minute)
	}

	return nil
}

func (s *SingleSnapController) isAppSetChanged(appResps []AppResponse) (isIdentical bool, tooAddSet *common.StringSet, tooDelSet *common.StringSet) {
	appSet := common.NewStringSet()
	for _, app := range appResps {
		if app.State == REGISTERED {
			appSet.Add(app.AppId)
		}
	}

	toDelSet := s.ApplicationSet.Minus(appSet)
	toAddSet := appSet.Minus(s.ApplicationSet)

	if appSet.IsIdentical(s.ApplicationSet) {
		s.ApplicationSet = appSet
		return false, toAddSet, toDelSet
	} else {
		s.ApplicationSet = appSet
		return true, toAddSet, toDelSet
	}
}

func (s *SingleSnapController) AppsUpdated(responses []AppResponse) {
	if err := s.SnapNode.TaskManager.isReady(); err != nil {
		log.Printf("[ AnalyzerPoller ] SnapNodes are not ready")
		return
	}

	var appToAdd *common.StringSet
	var appToDel *common.StringSet
	var isIdentical bool
	if isIdentical, appToAdd, appToDel = s.isAppSetChanged(responses); !isIdentical {
		return
	}

	log.Printf("[ AnalyzerPoller ] registered Application Set changed")
	log.Printf("[ AnalyzerPoller ] microservice of applications %s will be added to service list ", appToAdd.ToList())
	log.Printf("[ AnalyzerPoller ] microservice of applications %s will be deleted from service list", appToDel.ToList())

	for _, app := range responses {
		for _, svc := range app.Microservices {
			var pods []*v1.Pod
			switch svc.Kind {
			case "Deployment":
				hash, err := s.ClusterState.FindReplicaSetHash(svc.Name)
				if err != nil {
					log.Printf("[ AnalyzerPoller ] pod-template-hash is not found for deployment {%s}", svc.Name)
					continue
				}
				pods = s.ClusterState.FindDeploymentPod(svc.Namespace, svc.Name, hash)
			case "StatefulSet":
				pods = s.ClusterState.FindStatefulSetPod(svc.Namespace, svc.Name)
			default:
				log.Printf("[ AnalyzerPoller ] Not supported service kind {%s}", svc.Kind)
				continue
			}

			if appToAdd.IsExist(app.AppId) {
				s.ServiceList.add(app.AppId, svc.Name)
				snapNode := s.SnapNode
				for _, p := range pods {
					log.Printf("[ AnalyzerPoller ] add Running Service Pod {%s} in Node {%s}. ", p.Name, snapNode.NodeId)
					container := p.Spec.Containers[0]
					if ok := snapNode.RunningServicePods.find(p.Name); !ok {
						snapNode.RunningServicePods.addPodInfo(p.Name, ServicePodInfo{
							Namespace: p.Namespace,
							Port:      container.Ports[0].HostPort,
						})
					}
				}
			}

			if appToDel.IsExist(app.AppId) {
				s.ServiceList.deleteWholeApp(app.AppId)
				snapNode := s.SnapNode
				for _, p := range pods {
					log.Printf("[ AnalyzerPoller ] delete Running Service Pod {%s} in Node {%s}. ", p.Name, snapNode.NodeId)
					snapNode.RunningServicePods.delPodInfo(p.Name)
				}
			}

		}
	}

	s.SnapNode.reconcileSnapState()
}

func (s *SingleSnapController) createSnapNode() error {
	replicaClient := s.K8sClient.ExtensionsV1beta1Client.ReplicaSets(hyperpilotSnapNamespace)
	replicaList, err := replicaClient.List(metav1.ListOptions{})
	if err != nil {
		log.Printf("[ SingleSnapController ] List replicaSet fail: %s", err.Error())
		return err
	}

	hash := ""
	for _, replicaSet := range replicaList.Items {
		for _, ref := range replicaSet.OwnerReferences {
			if ref.Kind == "Deployment" && ref.Name == hyperpilotSnapDeploymentName {
				hash = replicaSet.ObjectMeta.Labels["pod-template-hash"]
			}
		}
	}
	if hash == "" {
		return errors.New(fmt.Sprintf("pod-template-hash label is not found for Deployment {%s}", hyperpilotSnapDeploymentName))
	}

	podClient := s.K8sClient.CoreV1Client.Pods(hyperpilotSnapNamespace)
	pods, err := podClient.List(metav1.ListOptions{
		LabelSelector: "pod-template-hash=" + hash,
	})

	if err != nil {
		log.Printf("[ SingleSnapController ] List Pod with Label pod-template-hash=%s fail: %s", hash, err.Error())
		return err
	}

	if len(pods.Items) == 0 {
		return errors.New(fmt.Sprintf("[ SingleSnapController ] can't find Pod with Label pod-template-hash=%s", hash))
	}
	nodeName := pods.Items[0].Spec.NodeName
	s.SnapNode = NewSnapNode(nodeName, s.ClusterState.Nodes[nodeName].ExternalIP, s.ServiceList, s.config)
	if err := s.SnapNode.initSingleSnap(s.isOutsideCluster(), &pods.Items[0], s.ClusterState); err != nil {
		log.Printf("[ SingleSnapController ] SnapNode Init fail : %s", err.Error())
		return err
	}
	return nil
}

func (s *SingleSnapController) isOutsideCluster() bool {
	return s.config.GetBool("Operator.OutsideCluster")
}

func (s *SingleSnapController) String() string {
	return fmt.Sprintf("SingleSnapController")
}

func (s *SingleSnapController) ProcessPod(e *common.PodEvent) {
	switch e.EventType {
	case common.DELETE:
		if e.Cur.Status.Phase != "Running" {
			return
		}
		if isHyperPilotSnapPod(e.Cur) {
			log.Printf("[ SingleSnapController ] Delete SnapNode in {%s}", s.DeletingSnapNode.NodeId)
			s.DeletingSnapNode.Exit()
		}
		if s.ServiceList.isServicePod(e.Cur) {
			s.SnapNode.PodEvents <- e
		}
	case common.ADD, common.UPDATE:
		if e.Cur.Status.Phase == "Running" && (e.Old == nil || e.Old.Status.Phase == "Pending") {
			nodeName := e.Cur.Spec.NodeName
			if isHyperPilotSnapPod(e.Cur) {
				if s.SnapNode != nil {
					s.DeletingSnapNode = s.SnapNode
				}
				newNode := NewSnapNode(nodeName, s.ClusterState.Nodes[nodeName].ExternalIP, s.ServiceList, s.config)
				s.SnapNode = newNode
				go func() {
					if err := s.SnapNode.initSingleSnap(s.isOutsideCluster(), e.Cur, s.ClusterState); err != nil {
						log.Printf("[ SingleSnapController ] Create new SnapNode ")
					}
				}()
			}
			if s.ServiceList.isServicePod(e.Cur) {
				s.SnapNode.PodEvents <- e
			}
		}
	}
}

func isHyperPilotSnapPod(pod *v1.Pod) bool {
	if strings.HasPrefix(pod.Name, hyperpilotSnapDeploymentName) {
		return true
	}
	return false
}

func (s *SingleSnapController) ProcessDeployment(e *common.DeploymentEvent) {}

func (s *SingleSnapController) ProcessDaemonSet(e *common.DaemonSetEvent) {}

func (s *SingleSnapController) ProcessNode(e *common.NodeEvent) {}

func (s *SingleSnapController) ProcessReplicaSet(e *common.ReplicaSetEvent) {}

func (s *SingleSnapController) Close() {
	/*
		deletePolicy := metav1.DeletePropagationForeground
		deploymentsClient := s.K8sClient.ExtensionsV1beta1Client.Deployments(hyperpilotSnapNamespace)
		if err := deploymentsClient.Delete(hyperpilotSnapDeploymentName, &metav1.DeleteOptions{
			PropagationPolicy: &deletePolicy,
		}); err != nil {
			log.Printf("[ SingleSnapController ] Delete deployment {%s} fail: %s", hyperpilotSnapDeploymentName, err.Error())
			return
		}
		log.Printf("[ SingleSnapController ] Delete deployment {%s}", hyperpilotSnapDeploymentName)
	*/
}
