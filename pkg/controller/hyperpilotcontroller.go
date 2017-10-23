package controller

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"log"
	"sync"
)

type Informer interface {
	onAdd()
	onDelete()
	onUpdate()
}

// HyperpilotController watches the kubernetes api for changes to Pods and
// delete completed Pods without specific annotation
type HyperpilotController struct {
	podInformer, deployInformer, daemonSetInformer cache.SharedIndexInformer
	//podInformer, deployInformer, daemonSetInformer Informer
	kclient     *kubernetes.Clientset
}


// NewHyperpilotController creates a new NewHyperpilotController
func NewHyperpilotController(kclient *kubernetes.Clientset, opts map[string]string) *HyperpilotController {
	hpc := &HyperpilotController{}
	hpc.kclient = kclient
	hpc.podInformer = InitPodInformer(kclient, opts)
	hpc.deployInformer = InitDeploymentInformer(kclient, opts)
	hpc.daemonSetInformer = InitDaemonSetInformer(kclient, opts)

	return hpc
}

// Run starts the process for listening for pod changes and acting upon those changes.
func (c *HyperpilotController) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	log.Printf("Listening for changes...")
	// When this function completes, mark the go function as done
	defer wg.Done()

	// Increment wait group as we're about to execute a go function
	wg.Add(1)

	// Execute go function
	go c.podInformer.Run(stopCh)
	go c.deployInformer.Run(stopCh)
	go c.daemonSetInformer.Run(stopCh)

	// Wait till we receive a stop signal
	<-stopCh
}


/*
func (c *HyperpilotController) doTheMagic(cur interface{}) {
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

*/

/*
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
*/