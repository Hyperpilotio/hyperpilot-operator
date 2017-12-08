package common

import (
	"errors"
	"log"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const HYPERPILOT_OPERATOR_NS = ""

type NodeInfo struct {
	NodeName   string
	ExternalIP string
	InternalIP string
}

type ClusterState struct {
	Nodes       map[string]NodeInfo
	Pods        map[string]*v1.Pod
	ReplicaSets map[string]*v1beta1.ReplicaSet
	Lock        *sync.RWMutex
}

func NewClusterState() *ClusterState {
	return &ClusterState{
		Nodes:       make(map[string]NodeInfo),
		Pods:        make(map[string]*v1.Pod),
		ReplicaSets: make(map[string]*v1beta1.ReplicaSet),
		Lock:        &sync.RWMutex{},
	}
}

func (clusterState *ClusterState) ProcessReplicaSet(e *ReplicaSetEvent) {
	clusterState.Lock.Lock()
	defer clusterState.Lock.Unlock()

	if e.EventType == ADD {
		if _, ok := clusterState.ReplicaSets[e.Cur.Name]; !ok {
			clusterState.ReplicaSets[e.Cur.Name] = e.Cur
			log.Printf("[ ClusterState ] Insert new ReplicaSet {%s}", e.Cur.Name)
		}
	}

	if e.EventType == DELETE {
		delete(clusterState.ReplicaSets, e.Cur.Name)
		log.Printf("[ ClusterState ] Delete ReplicaSet {%s},", e.Cur.Name)

	}
}

func (clusterState *ClusterState) ProcessPod(e *PodEvent) {
	clusterState.Lock.Lock()
	defer clusterState.Lock.Unlock()

	if e.EventType == DELETE {
		delete(clusterState.Pods, e.Cur.Name)
		log.Printf("[ ClusterState ] Delete Pod {%s}", e.Cur.Name)
	}

	// node info is available until pod is in running state
	if e.EventType == UPDATE {
		if e.Old.Status.Phase == "Pending" && (e.Cur.Status.Phase == "Running" || e.Cur.Status.Phase == "Succeeded") {
			clusterState.Pods[e.Cur.Name] = e.Cur

			log.Printf("[ ClusterState ] Insert NEW Pod {%s}", e.Cur.Name)
		}
	}
}

func (clusterState *ClusterState) ProcessNode(e *NodeEvent) {
	clusterState.Lock.Lock()
	defer clusterState.Lock.Unlock()

	if e.EventType == ADD {
		clusterState.Nodes[e.Cur.Name] = NodeInfo{
			NodeName:   e.Cur.Name,
			ExternalIP: e.Cur.Status.Addresses[1].Address,
			InternalIP: e.Cur.Status.Addresses[0].Address,
		}
		log.Printf("[ ClusterState ] Insert New Node {%s}", e.Cur.Name)
	}

	if e.EventType == DELETE {
		delete(clusterState.Nodes, e.Cur.Name)
		log.Printf("[ ClusterState ] Delete Node {%s}", e.Cur.Name)
	}
}

func (clusterState *ClusterState) PopulateNodeInfo(kclient *kubernetes.Clientset) error {
	nodes, err := kclient.Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	clusterState.Lock.Lock()
	defer clusterState.Lock.Unlock()

	for _, n := range nodes.Items {
		a := NodeInfo{
			NodeName:   n.Name,
			ExternalIP: n.Status.Addresses[1].Address,
			InternalIP: n.Status.Addresses[0].Address,
		}
		clusterState.Nodes[a.NodeName] = a
	}
	return nil
}

func (clusterState *ClusterState) PopulatePods(kclient *kubernetes.Clientset) error {
	pods, err := kclient.Pods(HYPERPILOT_OPERATOR_NS).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	clusterState.Lock.Lock()
	defer clusterState.Lock.Unlock()

	for _, p := range pods.Items {
		pod := p
		clusterState.Pods[pod.Name] = &pod
	}

	return nil
}

func (clusterState *ClusterState) PopulateReplicaSet(kclient *kubernetes.Clientset) error {
	rss, err := kclient.ReplicaSets(HYPERPILOT_OPERATOR_NS).List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	clusterState.Lock.Lock()
	defer clusterState.Lock.Unlock()

	for _, r := range rss.Items {
		rs := r
		clusterState.ReplicaSets[rs.Name] = &rs
	}
	return nil
}

func (clusterState *ClusterState) FindReplicaSetHash(deploymentName string) (string, error) {
	clusterState.Lock.RLock()
	defer clusterState.Lock.RUnlock()

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
	clusterState.Lock.RLock()
	defer clusterState.Lock.RUnlock()

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
	clusterState.Lock.RLock()
	defer clusterState.Lock.RUnlock()

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

func (clusterState *ClusterState) FindServicePod(namespace, service string) []*v1.Pod {
	clusterState.Lock.RLock()
	defer clusterState.Lock.RUnlock()

	r := []*v1.Pod{}
	for _, pod := range clusterState.Pods {

		r = append(r, pod)
	}

	return r
}

func (clusterState *ClusterState) FindPodRunningNodeInfo(podName string) NodeInfo {
	clusterState.Lock.RLock()
	defer clusterState.Lock.RUnlock()

	pod := clusterState.Pods[podName]
	return clusterState.Nodes[pod.Spec.NodeName]
}
