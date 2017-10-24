package controller

import (
	"k8s.io/client-go/pkg/api/v1"
	"strings"
	"fmt"
)

type Match interface {
	Evaluate(pod *v1.Pod) bool
}

type MatchNamePrefix struct{
	m Match
	Prefix string
}

func (p *MatchNamePrefix)Evaluate(pod *v1.Pod) bool  {
	return strings.HasPrefix(pod.Name, p.Prefix)
}

func (p *MatchNamePrefix) String() string {
	return fmt.Sprintf("MatchNamePrefix: {%v}", p.Prefix)
}

