package controller

import "k8s.io/client-go/pkg/api/v1"

type Match struct {
	podname string
}

func (m *Match)evaluate(pod *v1.Pod) bool  {
	return false
}