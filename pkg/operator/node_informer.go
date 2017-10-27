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

type NodeInformer struct {
	indexInformer cache.SharedIndexInformer
	hpc           *HyperpilotOpertor
}

func InitNodeInformer(kclient *kubernetes.Clientset, opts map[string]string, hpc *HyperpilotOpertor) NodeInformer {
	ni := NodeInformer{
		hpc: hpc,
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

func (ni *NodeInformer) onAdd(cur1 interface{}) {
	nodeObj := cur1.(*v1.Node)

	e := NodeEvent{
		ResourceEvent: ResourceEvent{
			Event_type: ADD,
		},
		Cur: nodeObj,
		Old: nil,
	}

	e.UpdateGlobalStatus()

	for _, ctr := range ni.hpc.nodeRegisters {
		go ctr.Receive(&e)
	}
}

func (ni *NodeInformer) onDelete(cur1 interface{}) {
	nodeObj := cur1.(*v1.Node)

	e := NodeEvent{
		ResourceEvent: ResourceEvent{
			Event_type: DELETE,
		},
		Cur: nodeObj,
		Old: nil,
	}
	e.UpdateGlobalStatus()

	for _, ctr := range ni.hpc.nodeRegisters {
		go ctr.Receive(&e)
	}
}

func (ni *NodeInformer) onUpdate(old, cur interface{}) {
	oldObj := old.(*v1.Node)
	curObj := cur.(*v1.Node)

	e := NodeEvent{
		ResourceEvent: ResourceEvent{
			Event_type: UPDATE,
		},
		Old: oldObj,
		Cur: curObj,
	}
	e.UpdateGlobalStatus()

	for _, ctr := range ni.hpc.nodeRegisters {
		go ctr.Receive(&e)
	}
}

//func (ni *NodeInformer) ProcessAddEvent(node *v1.Node) {
//	//log.Printf("Node name {%s}, Node Status {%s} ", node.Name, node.Status.Addresses )
//	log.Printf("%s", node.Status.Addresses[0].Type)
//	log.Printf("%s", node.Status.Addresses[0].Address)
//	//log.Printf("%s",node
//}
//
//func (ni *NodeInformer) ProcessDeleteEvent(node *v1.Node) {
//
//}
//
//func (ni *NodeInformer) ProcessUpdateEvent(node *v1.Node, node2 *v1.Node) {
//
//}
