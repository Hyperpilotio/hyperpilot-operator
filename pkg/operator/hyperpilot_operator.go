package operator

import (
	"k8s.io/client-go/kubernetes"
	"log"
	"sync"

	"database/sql"
	_ "github.com/proullon/ramsql/driver"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//const K8SEVENT = "K8SEVENT"

const (
	HYPERPILOT_OPERATOR_NS = "hyperpilot"
)

const (
	OPERATOR_INITIALIZING             = 0
	OPERATOR_INITIALIZING_CONTROLLERS = 1
	OPERATOR_RUNNING                  = 2
)

type NodeInfo struct {
	NodeName   string
	ExternalIP string
	InternalIP string
}

type PodInfo struct {
	PodName  string
	NodeId   string
	PodIP    string
	NodeName string
}

type HyperpilotOpertor struct {
	podInformer       *PodInformer
	deployInformer    *DeploymentInformer
	daemonSetInformer *DaemonSetInformer
	nodeInformer      *NodeInformer

	kclient *kubernetes.Clientset

	mu                 sync.Mutex
	podRegisters       []BaseController
	deployRegisters    []BaseController
	daemonSetRegisters []BaseController
	nsRegisters        []BaseController
	nodeRegisters      []BaseController

	controllers []BaseController
	// pod and node mapping
	db *sql.DB

	nodes map[string]NodeInfo
	pods  map[string]PodInfo

	state int
}

func (opertor *HyperpilotOpertor) GetWorld() {
	//
}

// NewHyperpilotOperator creates a new NewHyperpilotOperator
func NewHyperpilotOperator(kclient *kubernetes.Clientset, controllers []BaseController) *HyperpilotOpertor {
	hpc := &HyperpilotOpertor{
		podRegisters:       make([]BaseController, 0),
		deployRegisters:    make([]BaseController, 0),
		daemonSetRegisters: make([]BaseController, 0),
		nsRegisters:        make([]BaseController, 0),
		nodeRegisters:      make([]BaseController, 0),
		controllers:        controllers,
		kclient:            kclient,

		nodes: map[string]NodeInfo{},
		pods:  map[string]PodInfo{},
	}

	for _, controller := range controllers {
		resourceEnum := controller.GetResourceEnum()
		hpc.Accept(controller, resourceEnum)
	}

	return hpc
}

// Run starts the process for listening for pod changes and acting upon those changes.
func (c *HyperpilotOpertor) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	// Lifecycle:
	c.state = OPERATOR_INITIALIZING
	// 1. Register informers
	c.podInformer = InitPodInformer(c.kclient, c)
	//c.deployInformer = InitDeploymentInformer(c.kclient, c)
	//c.daemonSetInformer = InitDaemonSetInformer(c.kclient, c)
	c.nodeInformer = InitNodeInformer(c.kclient, c)

	// Execute go function
	go c.podInformer.indexInformer.Run(stopCh)
	//go c.deployInformer.indexInformer.Run(stopCh)
	//go c.daemonSetInformer.indexInformer.Run(stopCh)
	go c.nodeInformer.indexInformer.Run(stopCh)

	// 2. Initialize kubernetes state use KubeAPI get pods, .......
	nodes, _ := c.kclient.Nodes().List(metav1.ListOptions{})

	for _, n := range nodes.Items {
		a := NodeInfo{
			NodeName:   n.Name,
			ExternalIP: n.Status.Addresses[1].Address,
			InternalIP: n.Status.Addresses[0].Address,
		}
		c.nodes[a.NodeName] = a
	}
	for k, v := range c.nodes {
		log.Printf("{NodeName, Info}: {%s, %s}", k, v)
	}

	pods, _ := c.kclient.Pods(HYPERPILOT_OPERATOR_NS).List(metav1.ListOptions{})

	for _, p := range pods.Items {
		a := PodInfo{
			PodName:  p.Name,
			NodeId:   p.Spec.NodeName,
			PodIP:    p.Status.PodIP,
			NodeName: p.Spec.NodeName,
		}
		c.pods[a.PodName] = a
	}

	for k, v := range c.pods {
		log.Printf("{PodName, Info}: {%s, %s}", k, v)
	}

	// 3. Initialize controllers
	c.state = OPERATOR_INITIALIZING_CONTROLLERS
	controllerWg := &sync.WaitGroup{}
	for _, controller := range c.controllers {
		controllerWg.Add(1)
		go func() {
			controller.Init(c)
			controllerWg.Done()
		}()
	}

	controllerWg.Wait()
	// Use wait group to wait for all controller init to finish

	// 3. Forward events to controllers
	c.state = OPERATOR_RUNNING
	c.podInformer.onOperatorReady()
	//c.deployInformer.onOperatorReady()
	//c.daemonSetInformer.onOperatorReady()
	c.nodeInformer.onOperatorReady()

	// When this function completes, mark the go function as done
	defer wg.Done()
	// Increment wait group as we're about to execute a go function
	wg.Add(1)
	// Wait till we receive a stop signal
	<-stopCh
}

func (c *HyperpilotOpertor) Accept(s BaseController, res ResourceEnum) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if res.IsRegister(POD) {
		c.podRegisters = append(c.podRegisters, s)
		log.Printf("Contoller {%s} registered resource POD", s)
	}

	if res.IsRegister(DEPLOYMENT) {
		c.deployRegisters = append(c.deployRegisters, s)
		log.Printf("Contoller {%s} registered resource DEPLOYMENT", s)
	}

	if res.IsRegister(DAEMONSET) {
		c.daemonSetRegisters = append(c.daemonSetRegisters, s)
		log.Printf("Contoller {%s} registered resource DAEMONSET", s)
	}

	if res.IsRegister(NODE) {
		c.nodeRegisters = append(c.nodeRegisters, s)
		log.Printf("Contoller {%s} registered resource NODE", s)
	}

}
