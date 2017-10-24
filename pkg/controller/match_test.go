package controller

import (
	"testing"
	"k8s.io/client-go/pkg/api/v1"
	"fmt"
)

func TestPodNamePrefix(t *testing.T) {
	p := v1.Pod{}
	p.Name="abc"

	m:= MatchNamePrefix{}
	m.Prefix = "ab"

	if !m.Evaluate(&p) {
		t.Error("Expected 1.5", m.Prefix)
	}
}

func TestPodNamePrefix_String(t *testing.T) {
	m:= MatchNamePrefix{
		Prefix: "name123",
	}

	fmt.Print(m)
}



