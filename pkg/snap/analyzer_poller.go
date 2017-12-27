package snap

import (
	"fmt"
	"log"
	"time"

	"encoding/json"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/spf13/viper"
	"gopkg.in/resty.v1"
	"k8s.io/client-go/pkg/api/v1"
)

type AnalyzerPoller struct {
	config         *viper.Viper
	SnapController *SingleSnapController
	ApplicationSet *common.StringSet
}

func NewAnalyzerPoller(config *viper.Viper, snapController *SingleSnapController) *AnalyzerPoller {
	return &AnalyzerPoller{
		config:         config,
		SnapController: snapController,
		ApplicationSet: common.NewStringSet(),
	}
}

func (analyzerPoller *AnalyzerPoller) run() {
	tick := time.Tick(3 * time.Second)
	for {
		select {
		case <-tick:
			analyzerURL := fmt.Sprintf("%analyzerPoller%analyzerPoller%analyzerPoller%d%analyzerPoller",
				"http://", analyzerPoller.config.GetString("SnapTaskController.Analyzer.Address"),
				":", analyzerPoller.config.GetInt("SnapTaskController.Analyzer.Port"), API_APPS)
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
			analyzerPoller.checkApplications(appResp.Data)
		}
	}
}

func (analyzerPoller *AnalyzerPoller) checkApplications(appResps []AppResponse) {
	analyzerURL := fmt.Sprintf("%analyzerPoller%analyzerPoller%analyzerPoller%d%analyzerPoller",
		"http://", analyzerPoller.config.GetString("SnapTaskController.Analyzer.Address"),
		":", analyzerPoller.config.GetInt("SnapTaskController.Analyzer.Port"), API_K8SSERVICES)

	svcResp := ServiceResponse{}
	if !analyzerPoller.SnapController.SnapNode.TaskManager.isReady() {
		log.Printf("SnapNodes are not ready")
		return
	}
	if !analyzerPoller.isAppSetChange(appResps) {
		return
	}
	log.Printf("[ SnapTaskController ] Application Set change")

	// app set is empty
	if len(appResps) == 0 {
		pods := []*v1.Pod{}
		analyzerPoller.updateRunningServicePods(pods)
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
				hash, err := analyzerPoller.SnapController.ClusterState.FindReplicaSetHash(deployResponse.Data.Name)
				if err != nil {
					log.Printf("[ SnapTaskController ] pod-template-hash is not found for deployment {%analyzerPoller}", deployResponse.Data.Name)
					continue
				}
				pods := analyzerPoller.SnapController.ClusterState.FindDeploymentPod(deployResponse.Data.Namespace, deployResponse.Data.Name, hash)
				analyzerPoller.updateServiceList(deployResponse.Data.Name)
				analyzerPoller.updateRunningServicePods(pods)
			case "StatefulSet":
				statefulResponse := K8sStatefulSetResponse{}
				err = json.Unmarshal(resp.Body(), &statefulResponse)
				if err != nil {
					log.Print("JSON parse error: " + err.Error())
					return
				}
				pods := analyzerPoller.SnapController.ClusterState.FindStatefulSetPod(statefulResponse.Data.Namespace, statefulResponse.Data.Name)
				analyzerPoller.updateServiceList(statefulResponse.Data.Name)
				analyzerPoller.updateRunningServicePods(pods)
			default:
				log.Printf("Not supported service kind {%analyzerPoller}", svcResp.Kind)
			}
		}
	}
	analyzerPoller.SnapController.SnapNode.reconcileSnapState()
}

func (analyzerPoller *AnalyzerPoller) updateRunningServicePods(pods []*v1.Pod) {
	//add
	for _, p := range pods {
		snapNode := analyzerPoller.SnapController.SnapNode
		container := p.Spec.Containers[0]
		if _, ok := snapNode.RunningServicePods[p.Name]; !ok {
			snapNode.runningPodsMx.Lock()
			snapNode.RunningServicePods[p.Name] = ServicePodInfo{
				Namespace: p.Namespace,
				Port:      container.Ports[0].HostPort,
			}
			snapNode.runningPodsMx.Unlock()
			log.Printf("add Running Service Pod {%analyzerPoller} in Node {%analyzerPoller}. ", p.Name, snapNode.NodeId)
		}
	}

	//del
	for podName := range analyzerPoller.SnapController.SnapNode.RunningServicePods {
		if !containPod(pods, podName) {
			analyzerPoller.SnapController.SnapNode.runningPodsMx.Lock()
			delete(analyzerPoller.SnapController.SnapNode.RunningServicePods, podName)
			analyzerPoller.SnapController.SnapNode.runningPodsMx.Unlock()
			log.Printf("delete Running Service Pod {%analyzerPoller} in Node {%analyzerPoller} ", podName, analyzerPoller.SnapNode.NodeId)
		}
	}
}

// todo: lock!
// todo: check overwrite by analyzer result
func (analyzerPoller *AnalyzerPoller) updateServiceList(deployName string) {
	analyzerPoller.SnapController.ServiceList = append(analyzerPoller.SnapController.ServiceList, deployName)
}

func (analyzerPoller *AnalyzerPoller) isAppSetChange(appResps []AppResponse) bool {
	app_set := common.NewStringSet()
	for _, app := range appResps {
		app_set.Add(app.AppID)
	}

	if app_set.IsIdentical(analyzerPoller.ApplicationSet) {
		analyzerPoller.ApplicationSet = app_set
		return false
	} else {
		analyzerPoller.ApplicationSet = app_set
		return true
	}
}
