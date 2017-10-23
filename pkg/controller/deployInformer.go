package controller


import (
	"log"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
)

func InitDeploymentInformer(kclient *kubernetes.Clientset, opts map[string]string) cache.SharedIndexInformer{
	deployInformer :=cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kclient.ExtensionsV1beta1Client.Deployments(opts["namespace"]).List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kclient.ExtensionsV1beta1Client.Deployments(opts["namespace"]).Watch(options)
			},
		},
		&v1beta1.Deployment{},
		time.Second*30,
		cache.Indexers{},
	)

	deployInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(cur interface{}) {
			onDeploymentAdd(cur)
		},
		DeleteFunc: func(cur interface{}) {
			onDeploymentDelete(cur)
		},
		UpdateFunc: func(old, cur interface{}) {
			onDeploymentUpdate(old, cur)
		},

	})

	return deployInformer
}


func onDeploymentUpdate(i interface{}, i2 interface{}) {
	log.Printf("deploy update occur ")

}

func onDeploymentAdd(i interface{}) {
	deploy := i.(*v1beta1.Deployment)
	log.Printf("[DEPLOY] name: %s, namespace: %s", deploy.Name, deploy.Namespace)
}

func onDeploymentDelete(cur interface{})  {
	log.Printf("deploy DEL")
}
