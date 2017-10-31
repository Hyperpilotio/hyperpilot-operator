package operator

import (
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"log"
)

type EventType int

const (
	ADD    EventType = 1 << iota
	UPDATE EventType = 2
	DELETE EventType = 4
)

type ResourceEvent struct {
	Event_type EventType
}

type Event interface {
	GetType() EventType
	UpdateGlobalStatus(hpc *HyperpilotOpertor)
}

type NodeEvent struct {
	ResourceEvent
	Cur *v1.Node
	Old *v1.Node
}

func (r *NodeEvent) UpdateGlobalStatus(hpc *HyperpilotOpertor) {

}

func (r *NodeEvent) GetType() EventType {
	return r.Event_type
}

type PodEvent struct {
	ResourceEvent
	Cur *v1.Pod
	Old *v1.Pod
}

func (r *PodEvent) UpdateGlobalStatus(hpc *HyperpilotOpertor) {
	//delete pod info when delete event happens
	if r.Event_type == DELETE {
		delete(hpc.pods, r.Cur.Name)
		log.Printf("[ operator ] Delete Pod {%s}", r.Cur.Name)
	}

	// node info is available until pod is in running state
	if r.Event_type == UPDATE {
		if r.Old.Status.Phase == "Pending" && r.Cur.Status.Phase == "Running" {
			hpc.pods[r.Cur.Name] = PodInfo{
				PodName: r.Cur.Name,
				NodeId: r.Cur.Spec.NodeName,
				PodIP: r.Cur.Status.PodIP,
			}

			log.Printf("[ operator ] Insert NEW Pod {%s}", hpc.pods[r.Cur.Name])
		}
	}
}

func (r *PodEvent) GetType() EventType {
	return r.Event_type
}

type DeploymentEvent struct {
	ResourceEvent
	Old *v1beta1.Deployment
	Cur *v1beta1.Deployment
}

func (r *DeploymentEvent) GetType() EventType {
	return r.Event_type
}

func (r *DeploymentEvent) UpdateGlobalStatus(hpc *HyperpilotOpertor) {
	//TODO
	if r.Event_type == ADD {
	}

	//TODO
	if r.Event_type == DELETE {
	}

	//TODO
	if r.Event_type == UPDATE {
	}

}

type DaemonSetEvent struct {
	ResourceEvent
	Old *v1beta1.DaemonSet
	Cur *v1beta1.DaemonSet
}

func (r *DaemonSetEvent) GetType() EventType {
	return r.Event_type
}

func (r *DaemonSetEvent) UpdateGlobalStatus(hpc *HyperpilotOpertor) {

	//TODO
	if r.Event_type == ADD {
	}

	//TODO
	if r.Event_type == DELETE {
	}

	//TODO
	if r.Event_type == UPDATE {
	}
}
