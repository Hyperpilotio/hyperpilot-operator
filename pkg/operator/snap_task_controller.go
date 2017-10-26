package operator

import (
	"fmt"
	"github.com/emicklei/go-restful/log"
)

type SnapTaskController struct {}

func (s *SnapTaskController) Close() {

}

func (s *SnapTaskController) Register(hpc *HyperpilotOpertor, res ResourceEnum) {
	hpc.Accept(s, res)
}

func (s *SnapTaskController) Init() {

}

func (s *SnapTaskController) Receive(e Event) {

	_, ok := e.(*PodEvent)
	if ok {
		ProcessPod(e.(*PodEvent))
	}

	_, ok = e.(*DeploymentEvent)
	if ok {
		ProcessDeployment(e.(*DeploymentEvent))
	}

	_, ok = e.(*DaemonSetEvent)
	if ok {
		ProcessDaemonSet(e.(*DaemonSetEvent))
	}

}

func (s *SnapTaskController) String() string {
	return fmt.Sprintf("SnapTaskController")
}

func ProcessPod(e *PodEvent) {
	switch e.Event_type {
	case ADD:
		log.Printf("do pod add work")
	case DELETE:
		log.Printf("do pod delete work")
	case UPDATE:
		log.Printf("do pod update work")
	}
}

func ProcessDeployment(e *DeploymentEvent) {
	switch e.Event_type {
	case ADD:
		log.Printf("do deploy add work")
	case DELETE:
		log.Printf("do deploy delete work")
	case UPDATE:
		log.Printf("do deploy update work")
	}
}

func ProcessDaemonSet(e *DaemonSetEvent) {
	switch e.Event_type {
	case ADD:
		log.Printf("do daemonset add work")
	case DELETE:
		log.Printf("do daemonset delete work")
	case UPDATE:
		log.Printf("do daemonset update work")
	}
}
