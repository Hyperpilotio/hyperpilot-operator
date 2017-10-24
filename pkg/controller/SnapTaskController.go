package controller

import (
	"time"
	"log"

	"k8s.io/client-go/kubernetes"
	"github.com/satori/go.uuid"
)

type SnapTaskController struct {
	// controller id
	uuid uuid.UUID

	// match list
	matchList []Match

	// receive event from operator
	TodoEvents chan<- event

	// k8s client
	kclient     *kubernetes.Clientset

}

func NewSnapTask() (*SnapTaskController, error) {
	s := SnapTaskController{}
	s.uuid = uuid.NewV4()
	s.matchList = make([]Match, 5)

	r := register()
	if r == true {
		return &s, nil
	}else {
		return nil, nil
	}
}

func (s *SnapTaskController)Run(){


	// receive Snap Event and handle
	for event := range s.TodoEvents {
		event.handle()
	}

	for{
		log.Printf("UUID: %s \n", s.uuid)
		time.Sleep(time.Second * 5)
	}
}

func (s *SnapTaskController)Close()  {
	close(s.TodoEvents)
}

func register() bool{
	//send uuid

	//send matchList
	return false
}
