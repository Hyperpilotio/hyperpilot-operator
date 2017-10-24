package operator

import (
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"

	"log"
	"time"
)



type DaemonSetInformer struct{
	indexInformer cache.SharedIndexInformer
}

func InitDaemonSetInformer(kclient *kubernetes.Clientset, opts map[string]string) DaemonSetInformer{
	dsi := DaemonSetInformer{}

	daemonsetInformer :=cache.NewSharedIndexInformer(
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
	})

	dsi.indexInformer = daemonsetInformer
	return dsi
}


func (d *DaemonSetInformer)onAdd(i interface{}) {
	ds := i.(*v1beta1.DaemonSet)
	log.Printf("[DS] name: %s, namespace: %s", ds.Name, ds.Namespace)

}

func (d *DaemonSetInformer)onDelete(i interface{})  {

}

func (d *DaemonSetInformer)onUpdate(i,j interface{})  {

}



