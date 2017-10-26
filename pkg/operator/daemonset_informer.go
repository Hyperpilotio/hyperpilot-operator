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

type DaemonSetInformer struct {
	indexInformer cache.SharedIndexInformer
	hpc           *HyperpilotOpertor
}

func InitDaemonSetInformer(kclient *kubernetes.Clientset, opts map[string]string, hpc *HyperpilotOpertor) DaemonSetInformer {
	dsi := DaemonSetInformer{
		hpc: hpc,
	}

	daemonsetInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.ExtensionsV1beta1Client.DaemonSets(opts["namespace"]).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.ExtensionsV1beta1Client.DaemonSets(opts["namespace"]).Watch(options)
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

func (d *DaemonSetInformer) onAdd(i interface{}) {
	ds := i.(*v1beta1.DaemonSet)

	e := DaemonSetEvent{
		ResourceEvent: ResourceEvent{
			Event_type: ADD,
		},
		Cur: ds,
		Old: nil,
	}

	for _, ctr := range d.hpc.daemonSetRegisters {
		ctr.Receive(&e)
	}
}

func (d *DaemonSetInformer) onDelete(i interface{}) {
	ds := i.(*v1beta1.DaemonSet)

	e := DaemonSetEvent{
		ResourceEvent: ResourceEvent{
			Event_type: DELETE,
		},
		Cur: ds,
		Old: nil,
	}

	for _, ctr := range d.hpc.daemonSetRegisters {
		ctr.Receive(&e)
	}
}

func (d *DaemonSetInformer) onUpdate(i, j interface{}) {
	old := i.(*v1beta1.DaemonSet)
	cur := j.(*v1beta1.DaemonSet)

	e := DaemonSetEvent{
		ResourceEvent: ResourceEvent{
			Event_type: UPDATE,
		},
		Cur: cur,
		Old: old,
	}

	for _, ctr := range d.hpc.daemonSetRegisters {
		ctr.Receive(&e)
	}
}
