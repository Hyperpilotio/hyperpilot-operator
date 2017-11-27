package common

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

type Event interface {
	GetEventType() EventType
}

type ResourceEvent struct {
	EventType EventType
}

func (e ResourceEvent) GetEventType() EventType {
	return e.EventType
}

type NodeEvent struct {
	ResourceEvent
	Cur *v1.Node
	Old *v1.Node
}

type PodEvent struct {
	ResourceEvent
	Cur *v1.Pod
	Old *v1.Pod
}

type DeploymentEvent struct {
	ResourceEvent
	Old *v1beta1.Deployment
	Cur *v1beta1.Deployment
}

type DaemonSetEvent struct {
	ResourceEvent
	Old *v1beta1.DaemonSet
	Cur *v1beta1.DaemonSet
}

type ReplicaSetEvent struct {
	ResourceEvent
	Old *v1beta1.ReplicaSet
	Cur *v1beta1.ReplicaSet
}
