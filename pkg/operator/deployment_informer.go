package operator

import (
	"sync"
	"time"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

type DeploymentInformer struct {
	indexInformer cache.SharedIndexInformer
	processor     EventProcessor
	mutex         sync.Mutex
	queuedEvents  []*common.DeploymentEvent
	initializing  bool
}

func InitDeploymentInformer(kclient *kubernetes.Clientset, processor EventProcessor) *DeploymentInformer {
	di := &DeploymentInformer{
		processor:    processor,
		queuedEvents: []*common.DeploymentEvent{},
		initializing: true,
	}

	di.indexInformer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.ExtensionsV1beta1Client.Deployments(hyperpilotOperatorNamespace).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.ExtensionsV1beta1Client.Deployments(hyperpilotOperatorNamespace).Watch(options)
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
			d.processor.ProcessDeployment(e)
		}
	}
	// Clear queued events queue
	d.queuedEvents = d.queuedEvents[:0]

	d.initializing = false
}

func (d *DeploymentInformer) handleEvent(e *common.DeploymentEvent) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if d.initializing {
		d.queuedEvents = append(d.queuedEvents, e)
		return
	}

	d.processor.ProcessDeployment(e)
}

func (d *DeploymentInformer) onAdd(i interface{}) {
	deployObj := i.(*v1beta1.Deployment)

	e := &common.DeploymentEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.ADD,
		},
		Cur: deployObj,
		Old: nil,
	}

	d.handleEvent(e)
}

func (d *DeploymentInformer) onUpdate(i1 interface{}, i2 interface{}) {
	old := i1.(*v1beta1.Deployment)
	cur := i2.(*v1beta1.Deployment)

	e := &common.DeploymentEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.UPDATE,
		},
		Cur: cur,
		Old: old,
	}

	d.handleEvent(e)
}

func (d *DeploymentInformer) onDelete(cur interface{}) {
	deployObj := cur.(*v1beta1.Deployment)

	e := &common.DeploymentEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.DELETE,
		},
		Cur: deployObj,
		Old: nil,
	}

	d.handleEvent(e)
}
