package controller

import (
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"time"
)



func InitPodInformer(kclient *kubernetes.Clientset, opts map[string]string) cache.SharedIndexInformer{
	// Create informer for watching Pod
	podInformer := cache.NewSharedIndexInformer(
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
	podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		// can get  all existing pod when operator start
		// and
		// get new pod with pending status
		AddFunc: func(cur interface{}) {
			onPodAdd(cur)
		},
		DeleteFunc: func(cur interface{}) {
			onPodDelete(cur)
		},
		// get event when update, Pending -> Running  -> Successed
		//                                           |
		//                                            -> Failed
		UpdateFunc: func(old, cur interface{}) {
			oldPod := old.(*v1.Pod)
			newPod := cur.(*v1.Pod)
			if oldPod.Status.Phase == "Pending" &&
				newPod.Status.Phase =="Running"{
				onPodUpdate(old, cur)
			}
		},
	})

	return podInformer
}


func onPodAdd(cur interface{})  {
	podObj := cur.(*v1.Pod)

	if (podObj.Status.Phase == "Running"){
		log.Printf("Existing Pod")
		log.Printf("\t Pod: %s, \t NameSpace: %s, \t host: %s\n", podObj.Name ,podObj.Namespace, 	podObj.Spec.NodeName)
	}else if(podObj.Status.Phase == "Pending"){
		log.Printf("Newly crete Pod (in Pending Status)")
		log.Printf("\t Pod: %s, \t NameSpace: %s, \t host: %s \n", podObj.Name ,podObj.Namespace,	podObj.Spec.NodeName)
	}else if (podObj.Status.Phase == "Succeeded"){
		log.Printf("Completed Pod (in Successed Status)")
		log.Printf("\t Pod: %s, \t NameSpace: %s, \t host: %s \n", podObj.Name ,podObj.Namespace, podObj.Spec.NodeName)
	}else{
		log.Printf("Not defined Phase: %s", podObj.Status.Phase)
	}
}

func onPodDelete(cur interface{}){
	podObj := cur.(*v1.Pod)
	log.Printf("Deleting Pod")
	log.Printf("\t Pod: %s, \t NameSpace: %s, \t Status: %s \n",
		podObj.Name ,podObj.Namespace, podObj.Status.Phase)
}

func onPodUpdate(old, cur interface{})  {
	oldObj := old.(*v1.Pod)
	newObj := cur.(*v1.Pod)
	log.Printf("Newly crete Pod (in Running Status)")
	if oldObj.Status.Phase == "Pending" &&
		newObj.Status.Phase =="Running"{
		log.Printf("\t Pod: %s, \t NameSpace: %s, \t host: %s \n", newObj.Name ,newObj.Namespace,	newObj.Spec.NodeName)
	}

}




