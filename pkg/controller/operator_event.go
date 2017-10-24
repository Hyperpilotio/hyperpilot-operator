package controller

import "k8s.io/client-go/pkg/api/v1"

type event interface {
	handle()
}


type AddEvent struct {
	cur *v1.Pod
}

type DeleteEvent struct{
	cur *v1.Pod
}

type UpdateEvent struct{
	old *v1.Pod
	cur *v1.Pod
}


func (a *AddEvent)handle(){

}

func (d *DeleteEvent)handle()  {

}

func (u *UpdateEvent)handle()  {

}