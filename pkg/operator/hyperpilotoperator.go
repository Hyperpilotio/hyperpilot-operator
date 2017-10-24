package operator

import (
	"k8s.io/client-go/kubernetes"
	"log"
	"sync"
)

type Informer interface {
	//cache.SharedIndexInformer
	onAdd(interface{})
	onDelete(interface{})
	onUpdate(i,j interface{})
}

// HyperpilotOpertor watches the kubernetes api for changes to Pods and
// delete completed Pods without specific annotation
type HyperpilotOpertor struct {
	podInformer PodInformer
	deployInformer DeploymentInformer
	daemonSetInformer DaemonSetInformer
	kclient     *kubernetes.Clientset
	controller  *sync.Map
}


// NewHyperpilotOperator creates a new NewHyperpilotOperator
func NewHyperpilotOperator(kclient *kubernetes.Clientset, opts map[string]string) *HyperpilotOpertor {
	hpc := &HyperpilotOpertor{
		controller:  &sync.Map{},
		kclient: 	 kclient,
	}

	hpc.podInformer = InitPodInformer(kclient, opts, hpc.controller)
	hpc.deployInformer = InitDeploymentInformer(kclient, opts)
	hpc.daemonSetInformer = InitDaemonSetInformer(kclient, opts)
	return hpc
}

// Run starts the process for listening for pod changes and acting upon those changes.
func (c *HyperpilotOpertor) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	log.Printf("Listening for changes...")
	// When this function completes, mark the go function as done
	defer wg.Done()

	// Increment wait group as we're about to execute a go function
	wg.Add(1)

	// Execute go function
	go c.podInformer.indexInformer.Run(stopCh)
	//go c.deployInformer.indexInformer.Run(stopCh)
	//go c.daemonSetInformer.indexInformer.Run(stopCh)

	// Wait till we receive a stop signal
	<-stopCh
}


func (c *HyperpilotOpertor)Accept(s *SnapTaskController) {
	c.controller.Store(s.Uuid.String(), s)
	log.Printf("Controler {%s} registered. ", s.Uuid )
	log.Printf("Use following Match: {Condition}")
	for _, v := range s.MatchList {
		if v != nil {log.Printf("%s", v)}
	}
}

func (c *HyperpilotOpertor)Dcommission(id string) {
	c.controller.Delete(id)
}