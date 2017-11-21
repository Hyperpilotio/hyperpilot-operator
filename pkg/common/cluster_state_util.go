package common

import (
	"errors"
	"log"
	"strings"

	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type NodeInfo struct {
	NodeName   string
	ExternalIP string
	InternalIP string
}

type ClusterState struct {
	Nodes       map[string]NodeInfo
	Pods        map[string]*v1.Pod
	ReplicaSets map[string]*v1beta1.ReplicaSet
}

func (clusterState *ClusterState) FindReplicaSetHash(deploymentName string) (string, error) {
	for _, v := range clusterState.ReplicaSets {
		hash := v.ObjectMeta.Labels["pod-template-hash"]
		for _, owner := range v.OwnerReferences {
			if owner.Kind == "Deployment" && owner.Name == deploymentName {
				return hash, nil
			}
		}
	}
	return "", errors.New("[ ClusterState ] Can't find ReplicaSet Pod-template-hash for Deployment {" + deploymentName + "}")
}

func (clusterState *ClusterState) FindDeploymentPod(namespace, deploymentName, hash string) []*v1.Pod {
	r := []*v1.Pod{}
	for podName, pod := range clusterState.Pods {
		if strings.HasPrefix(podName, deploymentName) && strings.Contains(podName, hash) && pod.Namespace == namespace {
			p := &pod
			r = append(r, *p)
		}
	}
	if len(r) == 0 {
		log.Printf("[ ClusterState ] Can't find pod for Deployment {%s} in namespace {%s} ", deploymentName, namespace)
	}
	return r
}

func (clusterState *ClusterState) FindStatefulSetPod(namespace, statefulSetName string) []*v1.Pod {
	r := []*v1.Pod{}
	for _, pod := range clusterState.Pods {
		for _, own := range pod.OwnerReferences {
			if own.Kind == "StatefulSet" && own.Name == statefulSetName {
				p := &pod
				r = append(r, *p)
			}
		}
	}
	if len(r) == 0 {
		log.Printf("[ ClusterState ] Can't find pod for StatefulSet {%s} in namespace {%s} ", statefulSetName, namespace)
	}
	return r
}
