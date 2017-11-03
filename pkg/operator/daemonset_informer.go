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

type DaemonSetInformer struct {
	indexInformer cache.SharedIndexInformer
	processor     EventProcessor
	mutex         sync.Mutex
	queuedEvents  []*DaemonSetEvent
	initializing  bool
}

func InitDaemonSetInformer(kclient *kubernetes.Clientset, processor EventProcessor) *DaemonSetInformer {
	dsi := &DaemonSetInformer{
		processor:    processor,
		queuedEvents: []*DaemonSetEvent{},
		initializing: true,
	}

	daemonsetInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.ExtensionsV1beta1Client.DaemonSets(HYPERPILOT_OPERATOR_NS).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.ExtensionsV1beta1Client.DaemonSets(HYPERPILOT_OPERATOR_NS).Watch(options)
			},
		},
		&v1beta1.DaemonSet{},
		time.Second*30,
		cache.Indexers{},
	)

	daemonsetInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(cur interface{}) {
			dsi.onAdd(cur)
		},
		DeleteFunc: func(cur interface{}) {
			dsi.onDelete(cur)
		},
		UpdateFunc: func(old, cur interface{}) {
			dsi.onUpdate(old, cur)
		},
	})

	dsi.indexInformer = daemonsetInformer
	return dsi
}

func (d *DaemonSetInformer) onOperatorReady() {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if len(d.queuedEvents) > 0 {
		for _, e := range d.queuedEvents {
			d.processor.ProcessDaemonSet(e)
		}
	}
	// Clear queued events queue
	d.queuedEvents = d.queuedEvents[:0]

	d.initializing = false
}

func (d *DaemonSetInformer) handleEvent(e *DaemonSetEvent) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	if d.initializing {
		d.queuedEvents = append(d.queuedEvents, e)
		return
	}

	d.processor.ProcessDaemonSet(e)
}

func (d *DaemonSetInformer) onAdd(i interface{}) {
	ds := i.(*v1beta1.DaemonSet)

	e := &DaemonSetEvent{
		ResourceEvent: ResourceEvent{
			EventType: ADD,
		},
		Cur: ds,
		Old: nil,
	}

	d.handleEvent(e)
}

func (d *DaemonSetInformer) onDelete(i interface{}) {
	ds := i.(*v1beta1.DaemonSet)

	e := &DaemonSetEvent{
		ResourceEvent: ResourceEvent{
			EventType: DELETE,
		},
		Cur: ds,
		Old: nil,
	}

	d.handleEvent(e)
}

func (d *DaemonSetInformer) onUpdate(i, j interface{}) {
	old := i.(*v1beta1.DaemonSet)
	cur := j.(*v1beta1.DaemonSet)

	e := &DaemonSetEvent{
		ResourceEvent: ResourceEvent{
			EventType: UPDATE,
		},
		Cur: cur,
		Old: old,
	}

	d.handleEvent(e)
}
