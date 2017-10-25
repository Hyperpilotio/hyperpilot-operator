package operator

import (
	"github.com/hyperpilotio/hyperpilot-operator/pkg/controller/event"
)

type resourceEnum int

const (
	POD  		resourceEnum = 1 << iota
	DEPLOYMENT  resourceEnum = 2
	DAEMONSET 	resourceEnum = 4
	//NAMESPACE	resourceEnum = 8
)

func (this resourceEnum) IsRegister(flag resourceEnum) bool {
	return this|flag == this
}


type IController interface {
	Init()
	Register(hpc *HyperpilotOpertor, res resourceEnum)
	Receive(e event.Event)
	Close()
}


type BaseController struct{
	// receive event from operator
	// TodoEvents chan controller.Event
}



