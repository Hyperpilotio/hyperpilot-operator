package operator

import (
	"errors"
	"log"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

//const K8SEVENT = "K8SEVENT"

const (
	HYPERPILOT_OPERATOR_NS = "hyperpilot"

	// Operator states
	OPERATOR_NOT_RUNNING              = -1
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

	nodes map[string]NodeInfo
	pods  map[string]PodInfo

	state int
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
		nodes:              map[string]NodeInfo{},
		pods:               map[string]PodInfo{},
		state:              OPERATOR_NOT_RUNNING,
	}

	for _, controller := range controllers {
		resourceEnum := controller.GetResourceEnum()
		hpc.Accept(controller, resourceEnum)
	}

	return hpc
}

// Run starts the process for listening for pod changes and acting upon those changes.
func (c *HyperpilotOpertor) Run(stopCh <-chan struct{}) error {
	// Lifecycle:
	c.state = OPERATOR_INITIALIZING

	// 1. Register informers
	c.podInformer = InitPodInformer(c.kclient, c)
	//c.deployInformer = InitDeploymentInformer(c.kclient, c)
	//c.daemonSetInformer = InitDaemonSetInformer(c.kclient, c)
	c.nodeInformer = InitNodeInformer(c.kclient, c)

	go c.podInformer.indexInformer.Run(stopCh)
	//go c.deployInformer.indexInformer.Run(stopCh)
	//go c.daemonSetInformer.indexInformer.Run(stopCh)
	go c.nodeInformer.indexInformer.Run(stopCh)

	// 2. Initialize kubernetes state use KubeAPI get pods, .......
	nodes, err := c.kclient.Nodes().List(metav1.ListOptions{})
	if err != nil {
		return errors.New("Unable to list nodes from kubernetes: " + err.Error())
	}

	for _, n := range nodes.Items {
		a := NodeInfo{
			NodeName:   n.Name,
			ExternalIP: n.Status.Addresses[1].Address,
			InternalIP: n.Status.Addresses[0].Address,
		}
		c.nodes[a.NodeName] = a
	}

	pods, err := c.kclient.Pods(HYPERPILOT_OPERATOR_NS).List(metav1.ListOptions{})
	if err != nil {
		return errors.New("Unable to list pods from kubernetes")
	}

	for _, p := range pods.Items {
		a := PodInfo{
			PodName:  p.Name,
			NodeId:   p.Spec.NodeName,
			PodIP:    p.Status.PodIP,
			NodeName: p.Spec.NodeName,
		}
		c.pods[a.PodName] = a
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

	// Wait till we receive a stop signal
	<-stopCh

	return nil
}

func (c *HyperpilotOpertor) Accept(s BaseController, res ResourceEnum) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if res.IsRegistered(POD) {
		c.podRegisters = append(c.podRegisters, s)
		log.Printf("Contoller {%s} registered resource POD", s)
	}

	if res.IsRegistered(DEPLOYMENT) {
		c.deployRegisters = append(c.deployRegisters, s)
		log.Printf("Contoller {%s} registered resource DEPLOYMENT", s)
	}

	if res.IsRegistered(DAEMONSET) {
		c.daemonSetRegisters = append(c.daemonSetRegisters, s)
		log.Printf("Contoller {%s} registered resource DAEMONSET", s)
	}

	if res.IsRegistered(NODE) {
		c.nodeRegisters = append(c.nodeRegisters, s)
		log.Printf("Contoller {%s} registered resource NODE", s)
	}

}
