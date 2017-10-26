package operator

import (
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
}

func InitPodInformer(kclient *kubernetes.Clientset, opts map[string]string, hpc *HyperpilotOpertor) PodInformer {
	pi := PodInformer{
		hpc: hpc,
	}

	// Create informer for watching Pod
	pi.indexInformer = cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.CoreV1().Pods(opts["namespace"]).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.CoreV1().Pods(opts["namespace"]).Watch(options)
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

func (pi *PodInformer) onAdd(cur1 interface{}) {
	podObj := cur1.(*v1.Pod)

	e := PodEvent{
		ResourceEvent: ResourceEvent{
			Event_type: ADD,
		},
		Cur: podObj,
		Old: nil,
	}

	for _, ctr := range pi.hpc.podRegisters {
		go ctr.Receive(&e)
	}
}

func (pi *PodInformer) onDelete(cur interface{}) {
	podObj := cur.(*v1.Pod)

	e := PodEvent{
		ResourceEvent: ResourceEvent{
			Event_type: DELETE,
		},
		Cur: podObj,
		Old: nil,
	}
	for _, ctr := range pi.hpc.podRegisters {
		go ctr.Receive(&e)
	}

}

func (pi *PodInformer) onUpdate(old, cur interface{}) {
	oldObj := old.(*v1.Pod)
	curObj := cur.(*v1.Pod)

	e := PodEvent{
		ResourceEvent: ResourceEvent{
			Event_type: UPDATE,
		},
		Old: oldObj,
		Cur: curObj,
	}

	for _, ctr := range pi.hpc.podRegisters {
		go ctr.Receive(&e)
	}
}
