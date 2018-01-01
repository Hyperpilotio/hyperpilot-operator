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

	var appToAdd *common.StringSet
	var appToDel *common.StringSet
	var isIdentical bool
	if isIdentical, appToAdd, appToDel = analyzerPoller.isAppSetChanged(appResps); !isIdentical {
		return nil
	}
	log.Printf("[ AnalyzerPoller ] registered Application Set changed")
	log.Printf("[ AnalyzerPoller ] microservice of applications %s will be added to service list ", appToAdd.ToList())
	log.Printf("[ AnalyzerPoller ] microservice of applications %s will be deleted from service list", appToDel.ToList())

	for _, app := range appResps {
		for _, svc := range app.Microservices {
			var pods []*v1.Pod
			switch svc.Kind {
			case "Deployment":
				hash, err := analyzerPoller.SnapController.ClusterState.FindReplicaSetHash(svc.Name)
				if err != nil {
					log.Printf("[ AnalyzerPoller ] pod-template-hash is not found for deployment {%s}", svc.Name)
					continue
				}
				pods = analyzerPoller.SnapController.ClusterState.FindDeploymentPod(svc.Namespace, svc.Name, hash)
			case "StatefulSet":
				pods = analyzerPoller.SnapController.ClusterState.FindStatefulSetPod(svc.Namespace, svc.Name)
			default:
				log.Printf("[ AnalyzerPoller ] Not supported service kind {%s}", svc.Kind)
				continue
			}

			if appToAdd.IsExist(app.AppID) {
				analyzerPoller.SnapController.ServiceList.add(app.AppID, svc.Name)
				snapNode := analyzerPoller.SnapController.SnapNode
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

			if appToDel.IsExist(app.AppID) {
				analyzerPoller.SnapController.ServiceList.deleteWholeApp(app.AppID)
				snapNode := analyzerPoller.SnapController.SnapNode
				for _, p := range pods {
					log.Printf("[ AnalyzerPoller ] delete Running Service Pod {%s} in Node {%s}. ", p.Name, snapNode.NodeId)
					snapNode.RunningServicePods.delPodInfo(p.Name)
				}
			}

		}
	}
	return analyzerPoller.SnapController.SnapNode.reconcileSnapState()
}

func (analyzerPoller *AnalyzerPoller) isAppSetChanged(appResps []AppResponse) (isIdentical bool, tooAddSet *common.StringSet, tooDelSet *common.StringSet) {
	appSet := common.NewStringSet()
	for _, app := range appResps {
		if app.State == REGISTERED {
			appSet.Add(app.AppID)
		}
	}

	toDelSet := analyzerPoller.ApplicationSet.Minus(appSet)
	toAddSet := appSet.Minus(analyzerPoller.ApplicationSet)

	if appSet.IsIdentical(analyzerPoller.ApplicationSet) {
		analyzerPoller.ApplicationSet = appSet
		return false, toAddSet, toDelSet
	} else {
		analyzerPoller.ApplicationSet = appSet
		return true, toAddSet, toDelSet
	}
}

func (analyzerPoller *AnalyzerPoller) getEndpoint(path string) string {
	endpoint := fmt.Sprintf("%s%s%s%d%s",
		"http://", analyzerPoller.config.GetString("SnapTaskController.Analyzer.Address"),
		":", analyzerPoller.config.GetInt("SnapTaskController.Analyzer.Port"), path)
	return endpoint
}
