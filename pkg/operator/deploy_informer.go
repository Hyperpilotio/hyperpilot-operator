package operator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"time"
)

type DeploymentInformer struct {
	indexInformer cache.SharedIndexInformer
	hpc           *HyperpilotOpertor
}

func InitDeploymentInformer(kclient *kubernetes.Clientset, opts map[string]string, hpc *HyperpilotOpertor) DeploymentInformer {
	di := DeploymentInformer{
		hpc: hpc,
	}

	di.indexInformer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.ExtensionsV1beta1Client.Deployments(opts["namespace"]).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.ExtensionsV1beta1Client.Deployments(opts["namespace"]).Watch(options)
			},
		},
		&v1beta1.Deployment{},
		time.Second*30,
		cache.Indexers{},
	)

	di.indexInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(cur interface{}) {
			di.onAdd(cur)
		},
		DeleteFunc: func(cur interface{}) {
			di.onDelete(cur)
		},
		UpdateFunc: func(old, cur interface{}) {
			di.onUpdate(old, cur)
		},
	})

	return di
}

func (d *DeploymentInformer) onAdd(i interface{}) {
	deployObj := i.(*v1beta1.Deployment)

	e := DeploymentEvent{
		ResourceEvent: ResourceEvent{
			Event_type: ADD,
		},
		Cur: deployObj,
		Old: nil,
	}

	e.UpdateGlobalStatus()
	for _, ctr := range d.hpc.deployRegisters {
		go ctr.Receive(&e)
	}
}

func (d *DeploymentInformer) onUpdate(i1 interface{}, i2 interface{}) {
	old := i1.(*v1beta1.Deployment)
	cur := i2.(*v1beta1.Deployment)

	e := DeploymentEvent{
		ResourceEvent: ResourceEvent{
			Event_type: UPDATE,
		},
		Cur: cur,
		Old: old,
	}

	e.UpdateGlobalStatus()

	for _, ctr := range d.hpc.deployRegisters {
		go ctr.Receive(&e)
	}

}

func (d *DeploymentInformer) onDelete(cur interface{}) {
	deployObj := cur.(*v1beta1.Deployment)

	e := DeploymentEvent{
		ResourceEvent: ResourceEvent{
			Event_type: DELETE,
		},
		Cur: deployObj,
		Old: nil,
	}

	e.UpdateGlobalStatus()

	for _, ctr := range d.hpc.deployRegisters {
		go ctr.Receive(&e)
	}

}
