package controller

import (
	"encoding/json"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/cache"
	"log"
	//"reflect"
	"sync"
	"time"
)

// PodController watches the kubernetes api for changes to Pods and
// delete completed Pods without specific annotation
type PodController struct {
	podInformer cache.SharedIndexInformer
	kclient     *kubernetes.Clientset
}

type CreatedByAnnotation struct {
	Kind       string
	ApiVersion string
	Reference  struct {
		Kind            string
		Namespace       string
		Name            string
		Uid             string
		ApiVersion      string
		ResourceVersion string
	}
}

// NewPodController creates a new NewPodController
func NewPodController(kclient *kubernetes.Clientset, opts map[string]string) *PodController {
	podWatcher := &PodController{}

	// Create informer for watching Namespaces
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
			podWatcher.getAddInfo(cur)
		},
		DeleteFunc: func(cur interface{}) {
			podWatcher.getDelInfo(cur)
		},
		// get event when update, Pending -> Running  -> Successed
		//                                           |
		//                                            -> Failed
		UpdateFunc: func(old, cur interface{}) {
			//if !reflect.DeepEqual(old, cur) {
			//	podWatcher.getUpdateInfo(old, cur)
			//}

			oldPod := old.(*v1.Pod)
			newPod := cur.(*v1.Pod)
			if oldPod.Status.Phase == "Pending" &&
				newPod.Status.Phase =="Running"{
				podWatcher.getUpdateInfo(old, cur)
			}
		},
	})

	podWatcher.kclient = kclient
	podWatcher.podInformer = podInformer

	return podWatcher
}

// Run starts the process for listening for pod changes and acting upon those changes.
func (c *PodController) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	log.Printf("Listening for changes...")
	// When this function completes, mark the go function as done
	defer wg.Done()

	// Increment wait group as we're about to execute a go function
	wg.Add(1)

	// Execute go function
	go c.podInformer.Run(stopCh)

	// Wait till we receive a stop signal
	<-stopCh
}

func (c *PodController) doTheMagic(cur interface{}) {
	podObj := cur.(*v1.Pod)
	// Skip Pods in Running or Pending state
	if podObj.Status.Phase != "Succeeded" {
		return
	}
	var createdMeta CreatedByAnnotation
	json.Unmarshal([]byte(podObj.ObjectMeta.Annotations["kubernetes.io/created-by"]), &createdMeta)
	if createdMeta.Reference.Kind != "Job" {
		return
	}
	restartCounts := podObj.Status.ContainerStatuses[0].RestartCount
	if restartCounts == 0 {
		log.Printf("Going to delete pod '%s'", podObj.Name)
		// Delete Pod
		var po metav1.DeleteOptions
		c.kclient.CoreV1().Pods(podObj.Namespace).Delete(podObj.Name, &po)

		log.Printf("Going to delete job '%s'", createdMeta.Reference.Name)
		// Delete Job itself
		var jo metav1.DeleteOptions
		c.kclient.BatchV1Client.Jobs(createdMeta.Reference.Namespace).Delete(createdMeta.Reference.Name, &jo)
	}
}

func (c *PodController) getAddInfo(cur interface{})  {
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

func (c *PodController) getDelInfo(cur interface{}){
	podObj := cur.(*v1.Pod)
	log.Printf("Deleting Pod")
	log.Printf("\t Pod: %s, \t NameSpace: %s, \t Status: %s \n",
		podObj.Name ,podObj.Namespace, podObj.Status.Phase)
}

func (c *PodController) getUpdateInfo(old, cur interface{})  {
	oldObj := old.(*v1.Pod)
	newObj := cur.(*v1.Pod)
	log.Printf("Newly crete Pod (in Running Status)")
	if oldObj.Status.Phase == "Pending" &&
		newObj.Status.Phase =="Running"{
		log.Printf("\t Pod: %s, \t NameSpace: %s, \t host: %s \n", newObj.Name ,newObj.Namespace,	newObj.Spec.NodeName)
	}

}