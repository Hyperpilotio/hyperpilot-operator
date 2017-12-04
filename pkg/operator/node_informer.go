package operator

import (
	"sync"
	"time"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
)

type NodeInformer struct {
	indexInformer cache.SharedIndexInformer
	processor     EventProcessor
	mutex         sync.Mutex
	queuedEvents  []*common.NodeEvent
	initializing  bool
}

func InitNodeInformer(kclient *kubernetes.Clientset, processor EventProcessor) *NodeInformer {
	ni := &NodeInformer{
		processor:    processor,
		queuedEvents: []*common.NodeEvent{},
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
			ni.processor.ProcessNode(e)
		}
	}
	// Clear queued events queue
	ni.queuedEvents = ni.queuedEvents[:0]

	ni.initializing = false
}

func (ni *NodeInformer) handleEvent(e *common.NodeEvent) {
	ni.mutex.Lock()
	defer ni.mutex.Unlock()
	if ni.initializing {
		ni.queuedEvents = append(ni.queuedEvents, e)
		return
	}

	ni.processor.ProcessNode(e)
}

func (ni *NodeInformer) onAdd(cur interface{}) {
	nodeObj := cur.(*v1.Node)

	e := &common.NodeEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.ADD,
		},
		Cur: nodeObj,
		Old: nil,
	}

	ni.handleEvent(e)
}

func (ni *NodeInformer) onDelete(cur interface{}) {
	nodeObj := cur.(*v1.Node)

	e := &common.NodeEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.DELETE,
		},
		Cur: nodeObj,
		Old: nil,
	}

	ni.handleEvent(e)
}

func (ni *NodeInformer) onUpdate(old, cur interface{}) {
	oldObj := old.(*v1.Node)
	curObj := cur.(*v1.Node)

	e := &common.NodeEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.UPDATE,
		},
		Old: oldObj,
		Cur: curObj,
	}

	ni.handleEvent(e)
}
