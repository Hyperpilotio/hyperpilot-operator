package operator

import (
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
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
}

type PodEvent struct {
	ResourceEvent
	Cur *v1.Pod
	Old *v1.Pod
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

type DaemonSetEvent struct {
	ResourceEvent
	Old *v1beta1.DaemonSet
	Cur *v1beta1.DaemonSet
}

func (r *DaemonSetEvent) GetType() EventType {
	return r.Event_type
}
