package pod

import (
	"k8s.io/client-go/pkg/api/v1"
	"log"
)

type AddEvent struct {
	Obj *v1.Pod
}

type DeleteEvent struct{
	Obj *v1.Pod
}

type UpdateEvent struct{
	Old *v1.Pod
	Cur *v1.Pod
}


func (a *AddEvent)Handle(){
	if (a.Obj.Status.Phase == "Running"){
		log.Printf("Existing Pod")
		log.Printf("\t Pod: %s, \t NameSpace: %s, \t host: %s\n", a.Obj.Name ,a.Obj.Namespace, a.Obj.Spec.NodeName)
	}else if(a.Obj.Status.Phase == "Pending"){
		log.Printf("Newly crete Pod (in Pending Status)")
		log.Printf("\t Pod: %s, \t NameSpace: %s, \t host: %s \n", a.Obj.Name ,a.Obj.Namespace,	a.Obj.Spec.NodeName)
	}else if (a.Obj.Status.Phase == "Succeeded"){
		log.Printf("Completed Pod (in Successed Status)")
		log.Printf("\t Pod: %s, \t NameSpace: %s, \t host: %s \n", a.Obj.Name ,a.Obj.Namespace, a.Obj.Spec.NodeName)
	}else{
		log.Printf("Not defined Phase: %s", a.Obj.Status.Phase)
	}

}

func (a *AddEvent)String() string{
	return "add event"
}

func (d *DeleteEvent)Handle()  {

}

func (u *UpdateEvent)Handle()  {

}