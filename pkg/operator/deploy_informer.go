package operator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"
)

type DeploymentInformer struct {
	indexInformer cache.SharedIndexInformer
	hpc           *HyperpilotOpertor
	mutex         sync.Mutex
	queuedEvents  []*DeploymentEvent
	initializing  bool
}

func InitDeploymentInformer(kclient *kubernetes.Clientset, hpc *HyperpilotOpertor) *DeploymentInformer {
	di := &DeploymentInformer{
		hpc:          hpc,
		queuedEvents: []*DeploymentEvent{},
		initializing: true,
	}

	di.indexInformer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.ExtensionsV1beta1Client.Deployments(HYPERPILOT_OPERATOR_NS).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.ExtensionsV1beta1Client.Deployments(HYPERPILOT_OPERATOR_NS).Watch(options)
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

func (d *DeploymentInformer) onOperatorReady() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if len(d.queuedEvents) > 0 {

		for _, e := range d.queuedEvents {
			e.UpdateGlobalStatus(d.hpc)

			for _, ctr := range d.hpc.deployRegisters {
				go ctr.Receive(e)
			}
		}
	}
	// Clear queued events queue
	d.queuedEvents = d.queuedEvents[:0]

	d.initializing = false
}

func (d *DeploymentInformer) onAdd(i interface{}) {
	deployObj := i.(*v1beta1.Deployment)

	e := &DeploymentEvent{
		ResourceEvent: ResourceEvent{
			Event_type: ADD,
		},
		Cur: deployObj,
		Old: nil,
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.initializing {
		d.queuedEvents = append(d.queuedEvents, e)
		return
	}

	e.UpdateGlobalStatus(d.hpc)

	for _, ctr := range d.hpc.deployRegisters {
		go ctr.Receive(e)
	}
}

func (d *DeploymentInformer) onUpdate(i1 interface{}, i2 interface{}) {
	old := i1.(*v1beta1.Deployment)
	cur := i2.(*v1beta1.Deployment)

	e := &DeploymentEvent{
		ResourceEvent: ResourceEvent{
			Event_type: UPDATE,
		},
		Cur: cur,
		Old: old,
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.initializing {
		d.queuedEvents = append(d.queuedEvents, e)
		return
	}
	e.UpdateGlobalStatus(d.hpc)

	for _, ctr := range d.hpc.deployRegisters {
		go ctr.Receive(e)
	}

}

func (d *DeploymentInformer) onDelete(cur interface{}) {
	deployObj := cur.(*v1beta1.Deployment)

	e := &DeploymentEvent{
		ResourceEvent: ResourceEvent{
			Event_type: DELETE,
		},
		Cur: deployObj,
		Old: nil,
	}

	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.initializing {
		d.queuedEvents = append(d.queuedEvents, e)
		return
	}
	e.UpdateGlobalStatus(d.hpc)

	for _, ctr := range d.hpc.deployRegisters {
		go ctr.Receive(e)
	}

}
