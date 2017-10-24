package controller

import (
	"testing"
	"k8s.io/client-go/pkg/api/v1"
	"fmt"
)

func TestAddEvent(t *testing.T) {
	TodoEvents := make(chan Event)


	go func() {
		<-TodoEvents
	}()

	cur := v1.Pod{}
	e:= AddEvent{
		Obj: &cur,
	}

	TodoEvents <- &e

	fmt.Print("aaa")




}
