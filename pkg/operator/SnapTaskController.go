package operator

import (
	"log"

	"k8s.io/client-go/kubernetes"
	"github.com/satori/go.uuid"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/controller"
)

type SnapTaskController struct {
	// controller id
	Uuid uuid.UUID

	// match list
	MatchList []controller.Match

	// receive event from operator
	TodoEvents chan controller.Event

	hpc *HyperpilotOpertor

	// k8s client
	kclient     *kubernetes.Clientset

}

func NewSnapTask(kclient *kubernetes.Clientset, hpc *HyperpilotOpertor) (*SnapTaskController) {
	s := SnapTaskController{
		TodoEvents: make(chan controller.Event),
	}

	s.Uuid = uuid.NewV4()
	s.MatchList = buildMatchList()
	s.hpc = hpc
	s.kclient = kclient

	s.register()
	return &s
}

func buildMatchList() []controller.Match{
	//TODO: not hard code condition
	m1 := controller.MatchNamePrefix{
		Prefix:"influxsrv",
	}
	list := make([]controller.Match, 5)
	list = append(list, &m1)
	return list
}

func (s *SnapTaskController)Run(){

	log.Printf("Before hadnle event")
	for e:= range s.TodoEvents{
		e.Handle()
	}

	log.Printf("After hadnle event")
}

func (s *SnapTaskController)Close()  {
	close(s.TodoEvents)
	s.hpc.Dcommission(s.Uuid.String())
}

func (s *SnapTaskController)register() {
	s.hpc.Accept(s)
}
