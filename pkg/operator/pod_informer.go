package operator

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"time"
)

type PodInformer struct {
	indexInformer cache.SharedIndexInformer
	hpc           *HyperpilotOpertor
	mutex         sync.Mutex
	queuedEvents  []*PodEvent
	initializing  bool
}

func InitPodInformer(kclient *kubernetes.Clientset, hpc *HyperpilotOpertor) *PodInformer {
	pi := &PodInformer{
		hpc:          hpc,
		queuedEvents: []*PodEvent{},
		initializing: true,
	}

	// Create informer for watching Pod
	pi.indexInformer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				//return kclient.CoreV1().Pods(opts["namespace"]).List(options)
				return kclient.CoreV1().Pods(HYPERPILOT_OPERATOR_NS).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.CoreV1().Pods(HYPERPILOT_OPERATOR_NS).Watch(options)
			},
		},
		&v1.Pod{},
		time.Second*30,
		cache.Indexers{},
	)
	pi.indexInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(cur interface{}) {
			pi.onAdd(cur)
		},
		DeleteFunc: func(cur interface{}) {
			pi.onDelete(cur)
		},
		UpdateFunc: func(old, cur interface{}) {
			pi.onUpdate(old, cur)
		},
	})

	return pi
}

func (pi *PodInformer) onOperatorReady() {
	pi.mutex.Lock()
	defer pi.mutex.Unlock()

	if len(pi.queuedEvents) > 0 {

		for _, e := range pi.queuedEvents {
			e.UpdateGlobalStatus(pi.hpc)

			for _, ctr := range pi.hpc.podRegisters {
				go ctr.Receive(e)
			}
		}
	}
	// Clear queued events queue
	pi.queuedEvents = pi.queuedEvents[:0]

	pi.initializing = false
}

func (pi *PodInformer) onAdd(cur1 interface{}) {
	podObj := cur1.(*v1.Pod)

	e := &PodEvent{
		ResourceEvent: ResourceEvent{
			Event_type: ADD,
		},
		Cur: podObj,
		Old: nil,
	}

	pi.mutex.Lock()
	defer pi.mutex.Unlock()
	if pi.initializing {
		pi.queuedEvents = append(pi.queuedEvents, e)
		return
	}

	e.UpdateGlobalStatus(pi.hpc)

	for _, ctr := range pi.hpc.podRegisters {
		go ctr.Receive(e)
	}
}

func (pi *PodInformer) onDelete(cur interface{}) {
	podObj := cur.(*v1.Pod)

	e := &PodEvent{
		ResourceEvent: ResourceEvent{
			Event_type: DELETE,
		},
		Cur: podObj,
		Old: nil,
	}

	pi.mutex.Lock()
	defer pi.mutex.Unlock()
	if pi.initializing {
		pi.queuedEvents = append(pi.queuedEvents, e)
		return
	}
	e.UpdateGlobalStatus(pi.hpc)

	for _, ctr := range pi.hpc.podRegisters {
		go ctr.Receive(e)
	}

}

func (pi *PodInformer) onUpdate(old, cur interface{}) {
	oldObj := old.(*v1.Pod)
	curObj := cur.(*v1.Pod)

	e := &PodEvent{
		ResourceEvent: ResourceEvent{
			Event_type: UPDATE,
		},
		Old: oldObj,
		Cur: curObj,
	}

	pi.mutex.Lock()
	defer pi.mutex.Unlock()
	if pi.initializing {
		pi.queuedEvents = append(pi.queuedEvents, e)
		return
	}
	e.UpdateGlobalStatus(pi.hpc)

	for _, ctr := range pi.hpc.podRegisters {
		go ctr.Receive(e)
	}
}
