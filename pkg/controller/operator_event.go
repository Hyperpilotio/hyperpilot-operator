package controller

import (
	"k8s.io/client-go/pkg/api/v1"
	"log"
)

type Event interface {
	//TODO: add controller or k8s client in arguments
	Handle()
}


type AddEvent struct {
	Obj *v1.Pod
}

type DeleteEvent struct{
	cur *v1.Pod
}

type UpdateEvent struct{
	old *v1.Pod
	cur *v1.Pod
}


func (a *AddEvent)Handle(){
	log.Printf("handle event in handler")

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