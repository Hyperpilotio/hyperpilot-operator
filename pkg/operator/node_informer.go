package operator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"sync"
	"time"
)

type NodeInformer struct {
	indexInformer cache.SharedIndexInformer
	hpc           *HyperpilotOpertor
	mutex         sync.Mutex
	queuedEvents  []*NodeEvent
	initializing  bool
}

func InitNodeInformer(kclient *kubernetes.Clientset, hpc *HyperpilotOpertor) *NodeInformer {
	ni := &NodeInformer{
		hpc:          hpc,
		queuedEvents: []*NodeEvent{},
		initializing: true,
	}

	// Create informer for watching node
	ni.indexInformer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.CoreV1().Nodes().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.CoreV1().Nodes().Watch(options)
			},
		},
		&v1.Node{},
		time.Second*30,
		cache.Indexers{},
	)
	ni.indexInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(cur interface{}) {
			ni.onAdd(cur)
		},
		DeleteFunc: func(cur interface{}) {
			ni.onDelete(cur)
		},
		UpdateFunc: func(old, cur interface{}) {
			ni.onUpdate(old, cur)
		},
	})

	return ni

}

func (ni *NodeInformer) onOperatorReady() {
	ni.mutex.Lock()
	defer ni.mutex.Unlock()

	if len(ni.queuedEvents) > 0 {

		for _, e := range ni.queuedEvents {

			e.UpdateGlobalStatus(ni.hpc)

			for _, ctr := range ni.hpc.nodeRegisters {
				go ctr.Receive(e)
			}
		}
	}
	// Clear queued events queue
	ni.queuedEvents = ni.queuedEvents[:0]

	ni.initializing = false
}

func (ni *NodeInformer) onAdd(cur1 interface{}) {
	nodeObj := cur1.(*v1.Node)

	e := &NodeEvent{
		ResourceEvent: ResourceEvent{
			Event_type: ADD,
		},
		Cur: nodeObj,
		Old: nil,
	}

	ni.mutex.Lock()
	defer ni.mutex.Unlock()
	if ni.initializing {
		ni.queuedEvents = append(ni.queuedEvents, e)
		return
	}

	e.UpdateGlobalStatus(ni.hpc)
	for _, ctr := range ni.hpc.nodeRegisters {
		go ctr.Receive(e)
	}
}

func (ni *NodeInformer) onDelete(cur1 interface{}) {
	nodeObj := cur1.(*v1.Node)

	e := &NodeEvent{
		ResourceEvent: ResourceEvent{
			Event_type: DELETE,
		},
		Cur: nodeObj,
		Old: nil,
	}
	ni.mutex.Lock()
	defer ni.mutex.Unlock()
	if ni.initializing {
		ni.queuedEvents = append(ni.queuedEvents, e)
		return
	}

	e.UpdateGlobalStatus(ni.hpc)
	for _, ctr := range ni.hpc.nodeRegisters {
		go ctr.Receive(e)
	}
}

func (ni *NodeInformer) onUpdate(old, cur interface{}) {
	oldObj := old.(*v1.Node)
	curObj := cur.(*v1.Node)

	e := &NodeEvent{
		ResourceEvent: ResourceEvent{
			Event_type: UPDATE,
		},
		Old: oldObj,
		Cur: curObj,
	}
	ni.mutex.Lock()
	defer ni.mutex.Unlock()
	if ni.initializing {
		ni.queuedEvents = append(ni.queuedEvents, e)
		return
	}
	e.UpdateGlobalStatus(ni.hpc)

	for _, ctr := range ni.hpc.nodeRegisters {
		go ctr.Receive(e)
	}
}
