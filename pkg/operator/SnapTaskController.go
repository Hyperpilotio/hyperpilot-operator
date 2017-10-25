package operator

import (
	"k8s.io/client-go/kubernetes"
	"fmt"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/controller/event"
)

type SnapTaskController struct {
	BaseController

	// k8s client
	//kclient     *kubernetes.Clientset
}

func NewSnapTask(kclient *kubernetes.Clientset, hpc *HyperpilotOpertor, res resourceEnum) (*SnapTaskController) {
	s := SnapTaskController{
		BaseController: BaseController{},
	}
	//s.kclient = kclient

	s.Register(hpc, res)
	return &s
}


func (s *SnapTaskController)Close()  {

}


func (s *SnapTaskController) Register(hpc *HyperpilotOpertor, res resourceEnum) {
	hpc.Accept(s, res)
}

func (s *SnapTaskController) Init(){

}

func (s *SnapTaskController) Receive(event event.Event){
	event.Handle()
}

func (s *SnapTaskController)String() string{
	return fmt.Sprintf("SnapTaskController")
}