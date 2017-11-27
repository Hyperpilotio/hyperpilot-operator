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

type ReplicaSetInformer struct {
	indexInformer cache.SharedIndexInformer
	processor     EventProcessor
	mutex         sync.Mutex
	queuedEvents  []*common.ReplicaSetEvent
	initializing  bool
}

func InitReplicaSetInformer(kclient *kubernetes.Clientset, processor EventProcessor) *ReplicaSetInformer {
	r := &ReplicaSetInformer{
		processor:    processor,
		queuedEvents: []*common.ReplicaSetEvent{},
		initializing: true,
	}

	// Create informer for watching Pod
	r.indexInformer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				//return kclient.CoreV1().Pods(opts["namespace"]).List(options)
				return kclient.ExtensionsV1beta1Client.ReplicaSets(HYPERPILOT_OPERATOR_NS).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.ExtensionsV1beta1Client.ReplicaSets(HYPERPILOT_OPERATOR_NS).Watch(options)
			},
		},
		&v1beta1.ReplicaSet{},
		time.Second*30,
		cache.Indexers{},
	)
	r.indexInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(cur interface{}) {
			r.onAdd(cur)
		},
		DeleteFunc: func(cur interface{}) {
			r.onDelete(cur)
		},
		UpdateFunc: func(old, cur interface{}) {
			r.onUpdate(old, cur)
		},
	})

	return r
}

func (r *ReplicaSetInformer) onOperatorReady() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if len(r.queuedEvents) > 0 {
		for _, e := range r.queuedEvents {
			r.processor.ProcessReplicaSet(e)
		}
	}
	// Clear queued events queue
	r.queuedEvents = r.queuedEvents[:0]

	r.initializing = false
}

func (r *ReplicaSetInformer) handleEvent(e *common.ReplicaSetEvent) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.initializing {
		r.queuedEvents = append(r.queuedEvents, e)
		return
	}

	r.processor.ProcessReplicaSet(e)
}

func (r *ReplicaSetInformer) onAdd(cur1 interface{}) {
	podObj := cur1.(*v1beta1.ReplicaSet)

	e := &common.ReplicaSetEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.ADD,
		},
		Cur: podObj,
		Old: nil,
	}

	r.handleEvent(e)
}

func (r *ReplicaSetInformer) onDelete(cur interface{}) {
	podObj := cur.(*v1beta1.ReplicaSet)

	e := &common.ReplicaSetEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.DELETE,
		},
		Cur: podObj,
		Old: nil,
	}

	r.handleEvent(e)
}

func (r *ReplicaSetInformer) onUpdate(old, cur interface{}) {
	oldObj := old.(*v1beta1.ReplicaSet)
	curObj := cur.(*v1beta1.ReplicaSet)

	e := &common.ReplicaSetEvent{
		ResourceEvent: common.ResourceEvent{
			EventType: common.UPDATE,
		},
		Old: oldObj,
		Cur: curObj,
	}

	r.handleEvent(e)
}
