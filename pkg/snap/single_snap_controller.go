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
	SnapExternalIP   string
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

func (watchinglist *ServiceWatchingList) findAppIDOfMicroservice(serviceName string) string {
	watchinglist.Lock.Lock()
	defer watchinglist.Lock.Unlock()

	for appID, serviceList := range watchinglist.watchingList {
		for _, service := range serviceList {
			if strings.HasPrefix(serviceName, service) {
				return appID
			}
		}
	}
	return ""
}

func (watchinglist *ServiceWatchingList) add(appID, serviceName string) {
	watchinglist.Lock.Lock()
	defer watchinglist.Lock.Unlock()

	if _, ok := watchinglist.watchingList[appID]; !ok {
		watchinglist.watchingList[appID] = []string{}
	}

	services := watchinglist.watchingList[appID]
	for _, service := range services {
		if service == serviceName {
			log.Printf("Skip adding service %s to app %s watch list as it already exists", serviceName, appID)
			return
		}
	}

	watchinglist.watchingList[appID] = append(services, serviceName)
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
	serviceClient := kclient.CoreV1Client.Services(hyperpilotSnapNamespace)

	if !common.HasService(s.K8sClient, hyperpilotSnapNamespace, hyperpilotSnapDeploymentName) {
		var svcType v1.ServiceType
		if s.isOutsideCluster() {
			svcType = v1.ServiceTypeLoadBalancer
		} else {
			svcType = v1.ServiceTypeClusterIP
		}
		if err := common.CreateService(s.K8sClient, hyperpilotSnapNamespace, hyperpilotSnapDeploymentName,
			svcType, []int32{int32(8181)}, []int32{int32(8181)}); err != nil {
			return errors.New("Unable to create service for hyperpilot snap: " + err.Error())
		}
	}

	// wait for external IP available
	if s.isOutsideCluster() {
		for {
			log.Printf("[ SingleSnapController ] operator run outside cluster, wait for external IP for Snap")
			svc, err := serviceClient.Get(hyperpilotSnapDeploymentName, metav1.GetOptions{})

			if err != nil {
				log.Printf("[ SingleSnapController ] Get Service {%s} status fail", hyperpilotSnapDeploymentName)
				return err
			}

			if len(svc.Status.LoadBalancer.Ingress) > 0 {
				log.Printf("[ SingleSnapController ] Obtain external IP=%s", svc.Status.LoadBalancer.Ingress[0].IP)
				s.SnapExternalIP = svc.Status.LoadBalancer.Ingress[0].IP
				break
			}
			time.Sleep(20 * time.Second)
		}
	}

	// create deployment if not exist
	if !common.HasDeployment(s.K8sClient, hyperpilotSnapNamespace, hyperpilotSnapDeploymentName) {
		log.Printf("[ SingleSnapController ] deployment {%s} is not exist, create new one", hyperpilotSnapDeploymentName)
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
	}

	// create SnapNode util deployment is available
	for {
		log.Printf("[ SingleSnapController ] Chechk deployment {%s} status, and wait status become available", hyperpilotSnapDeploymentName)
		d, err := deployClient.Get(hyperpilotSnapDeploymentName, metav1.GetOptions{})
		if err != nil {
			log.Printf("[ SingleSnapController ] Get deployment {%s} status fail: %s ", hyperpilotSnapDeploymentName, err.Error())
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
		time.Sleep(5 * time.Second)
	}

	if s.config.GetBool("SnapTaskController.Analyzer.Enable") {
		log.Printf("[ SnapTaskController ] Poll Analyzer flag is enabled, launch goroutin to poll analyzer")
		s.analyzerPoller = NewAnalyzerPoller(s.config, s, 10*time.Second, 3*time.Minute)
	}

	return nil
}

func (s *SingleSnapController) isAppSetChanged(appResps []AppResponse) (isIdentical bool, tooAddSet *common.StringSet, tooDelSet *common.StringSet) {
	appSet := common.NewStringSet()
	for _, app := range appResps {
		if app.State == ACTIVE {
			appSet.Add(app.AppId)
		}
	}

	toDelSet := s.ApplicationSet.Minus(appSet)
	toAddSet := appSet.Minus(s.ApplicationSet)

	if appSet.IsIdentical(s.ApplicationSet) {
		return false, toAddSet, toDelSet
	}

	s.ApplicationSet = appSet
	return true, toAddSet, toDelSet
}

func (s *SingleSnapController) AppsUpdated(responses []AppResponse) {
	if err := s.SnapNode.TaskManager.isReady(); err != nil {
		log.Printf("[ SingleSnapController ] SnapNodes are not ready, skipping update")
		return
	}

	var appToAdd *common.StringSet
	var appToDel *common.StringSet
	var isChanged bool
	if isChanged, appToAdd, appToDel = s.isAppSetChanged(responses); !isChanged {
		log.Printf("[ SingleSnapController ] No apps changed, skipping update")
		return
	}

	log.Printf("[ SingleSnapController ] registered Application Set changed")
	log.Printf("[ SingleSnapController ] microservice of applications %s will be added to service list ", appToAdd.ToList())
	log.Printf("[ SingleSnapController ] microservice of applications %s will be deleted from service list", appToDel.ToList())

	for _, app := range responses {
		source := app.SLO.Source
		if source.APMType != "prometheus" {
			log.Printf("[ SingleSnapController ] skip adding app %s as it's APM type is not supported: %s", app.AppId, source.APMType)
			continue
		}

		svc := source.Service
		var pods []*v1.Pod
		switch svc.Kind {
		case "Deployment", "deployments":
			hash, err := s.ClusterState.FindReplicaSetHash(svc.Name)
			if err != nil {
				log.Printf("[ SingleSnapController ] pod-template-hash is not found for deployment {%s}", svc.Name)
				continue
			}
			pods = s.ClusterState.FindDeploymentPod(svc.Namespace, svc.Name, hash)
		case "StatefulSet", "statefulsets":
			pods = s.ClusterState.FindStatefulSetPod(svc.Namespace, svc.Name)
		default:
			log.Printf("[ SingleSnapController ] Not supported service kind {%s}", svc.Kind)
			continue
		}

		if appToAdd.IsExist(app.AppId) {
			s.ServiceList.add(app.AppId, svc.Name)
			snapNode := s.SnapNode
			for _, p := range pods {
				log.Printf("[ SingleSnapController ] add Running Service Pod {%s}. ", p.Name)
				container := p.Spec.Containers[0]
				if ok := snapNode.RunningServicePods.find(p.Name); !ok {
					snapNode.RunningServicePods.addPodInfo(p.Name, ServicePodInfo{
						Namespace: p.Namespace,
						Port:      container.Ports[0].HostPort,
						PodIP:     p.Status.PodIP,
						PodName:   p.Name,
						Appid:     app.AppId,
					})
				}
			}
		} else if appToDel.IsExist(app.AppId) {
			s.ServiceList.deleteWholeApp(app.AppId)
			snapNode := s.SnapNode
			for _, p := range pods {
				log.Printf("[ SingleSnapController ] delete Running Service Pod {%s}. ", p.Name)
				snapNode.RunningServicePods.delPodInfo(p.Name)
			}
		}

	}

	s.SnapNode.reconcileSnapState()
}

func (s *SingleSnapController) createSnapNode() error {

	if s.isOutsideCluster() {
		s.SnapNode = NewSnapNode(s.SnapExternalIP, s.ServiceList, s.config)
	} else {
		s.SnapNode = NewSnapNode(hyperpilotSnapDeploymentName, s.ServiceList, s.config)
	}

	if err := s.SnapNode.initSingleSnap(s.ClusterState); err != nil {
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
			log.Printf("[ SingleSnapController ] SnapNode is Deleted")
			s.DeletingSnapNode.Exit()
		}
		if s.ServiceList.isServicePod(e.Cur) {
			s.SnapNode.PodEvents <- e
		}
	case common.ADD, common.UPDATE:
		if e.Cur.Status.Phase == "Running" && (e.Old == nil || e.Old.Status.Phase == "Pending") {
			if isHyperPilotSnapPod(e.Cur) {
				if s.SnapNode != nil {
					s.DeletingSnapNode = s.SnapNode
				}
				if s.isOutsideCluster() {
					s.SnapNode = NewSnapNode(s.SnapExternalIP, s.ServiceList, s.config)
				} else {
					s.SnapNode = NewSnapNode(hyperpilotSnapDeploymentName, s.ServiceList, s.config)
				}
				go func() {
					if err := s.SnapNode.initSingleSnap(s.ClusterState); err != nil {
						log.Printf("[ SingleSnapController ] Create new SnapNode fail: " + err.Error())
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
