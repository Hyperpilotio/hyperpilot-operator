package daemonset

import(
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"log"
)

type AddEvent struct {
	Obj *v1beta1.DaemonSet
}

type DeleteEvent struct{
	Cur *v1beta1.DaemonSet
}

type UpdateEvent struct{
	Old *v1beta1.DaemonSet
	Cur *v1beta1.DaemonSet
}


func (a *AddEvent)Handle(){
	log.Printf("handle daemonset event in handler")

}

func (a *AddEvent)String() string{
	return "daemonset add event"
}

func (d *DeleteEvent)Handle()  {

}

func (u *UpdateEvent)Handle()  {

}
