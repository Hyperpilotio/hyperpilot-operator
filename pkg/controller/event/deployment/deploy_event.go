package deployment

import(
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"log"
)

type AddEvent struct {
	Obj *v1beta1.Deployment
}

type DeleteEvent struct{
	Cur *v1beta1.Deployment
}

type UpdateEvent struct{
	Old *v1beta1.Deployment
	Cur *v1beta1.Deployment
}


func (a *AddEvent)Handle(){
	log.Printf("handle deploy event in handler")

}

func (a *AddEvent)String() string{
	return "deploy add event"
}

func (d *DeleteEvent)Handle()  {

}

func (u *UpdateEvent)Handle()  {

}
