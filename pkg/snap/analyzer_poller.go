package snap

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff"
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
	tick := time.Tick(1 * time.Minute)
	for {
		select {
		case <-tick:
			b := backoff.NewExponentialBackOff()
			b.MaxElapsedTime = 1 * time.Minute
			err := backoff.Retry(analyzerPoller.poll, b)
			if err != nil {
				log.Printf("[ AnalyzerPoller ] Polling to Analyzer fail after Retry: %s", err.Error())
			}
		}
	}
}

func (analyzerPoller *AnalyzerPoller) poll() error {
	analyzerURL := analyzerPoller.getEndpoint(apiApps)
	appResp := AppResponses{}
	resp, err := resty.R().Get(analyzerURL)
	if err != nil {
		log.Printf("[ AnalyzerPoller ] GET all apps from url {%s} error: %s", analyzerURL, err.Error())
		return err
	}
	err = json.Unmarshal(resp.Body(), &appResp)
	if err != nil {
		log.Printf("[ AnalyzerPoller ] Unable unmarshal JSON response from {%s} to AppResponses: %s", analyzerURL, err.Error())
		return err
	}
	return analyzerPoller.checkApplications(appResp.Data)
}

func (analyzerPoller *AnalyzerPoller) checkApplications(appResps []AppResponse) error {
	if err := analyzerPoller.SnapController.SnapNode.TaskManager.isReady(); err != nil {
		log.Printf("[ AnalyzerPoller ] SnapNodes are not ready")
		return nil
	}
	if !analyzerPoller.isAppSetChanged(appResps) {
		return nil
	}
	log.Printf("[ AnalyzerPoller ] Application Set change")

	// app set is empty
	if len(appResps) == 0 {
		pods := []*v1.Pod{}
		analyzerPoller.updateRunningServicePods(pods)
	}

	// app set not empty
	for _, app := range appResps {
		for _, svc := range app.Microservices {
			switch svc.Kind {
			case "Deployment":
				hash, err := analyzerPoller.SnapController.ClusterState.FindReplicaSetHash(svc.Name)
				if err != nil {
					log.Printf("[ AnalyzerPoller ] pod-template-hash is not found for deployment {%s}", svc.Name)
					continue
				}
				pods := analyzerPoller.SnapController.ClusterState.FindDeploymentPod(svc.Namespace, svc.Name, hash)
				analyzerPoller.updateServiceList(svc.Name)
				analyzerPoller.updateRunningServicePods(pods)
			case "StatefulSet":
				pods := analyzerPoller.SnapController.ClusterState.FindStatefulSetPod(svc.Namespace, svc.Name)
				analyzerPoller.updateServiceList(svc.Name)
				analyzerPoller.updateRunningServicePods(pods)
			default:
				log.Printf("[ AnalyzerPoller ] Not supported service kind {%s}", svc.Kind)
			}
		}
	}
	return analyzerPoller.SnapController.SnapNode.reconcileSnapState()
}

func (analyzerPoller *AnalyzerPoller) updateRunningServicePods(pods []*v1.Pod) {
	snapNode := analyzerPoller.SnapController.SnapNode

	//1. add Pod to RunningServicePods when pod is exist
	for _, p := range pods {
		container := p.Spec.Containers[0]

		if ok := snapNode.RunningServicePods.find(p.Name); !ok {
			snapNode.RunningServicePods.addPodInfo(p.Name, ServicePodInfo{
				Namespace: p.Namespace,
				Port:      container.Ports[0].HostPort,
			})
			log.Printf("add Running Service Pod {%s} in Node {%s}. ", p.Name, snapNode.NodeId)
		}
	}

	//2. delete pod from RunningServicePods when the pod is not exist
	snapNode.RunningServicePods.deletePodInfoIfNotPresentInList(pods)
}

// todo: lock!
// todo: check overwrite by analyzer result
func (analyzerPoller *AnalyzerPoller) updateServiceList(deployName string) {
	analyzerPoller.SnapController.ServiceList = append(analyzerPoller.SnapController.ServiceList, deployName)
}

func (analyzerPoller *AnalyzerPoller) isAppSetChanged(appResps []AppResponse) bool {
	appSet := common.NewStringSet()
	for _, app := range appResps {
		appSet.Add(app.AppID)
	}

	if appSet.IsIdentical(analyzerPoller.ApplicationSet) {
		analyzerPoller.ApplicationSet = appSet
		return false
	} else {
		analyzerPoller.ApplicationSet = appSet
		return true
	}
}

func (analyzerPoller *AnalyzerPoller) getEndpoint(path string) string {
	endpoint := fmt.Sprintf("%s%s%s%d%s",
		"http://", analyzerPoller.config.GetString("SnapTaskController.Analyzer.Address"),
		":", analyzerPoller.config.GetInt("SnapTaskController.Analyzer.Port"), path)
	return endpoint
}
