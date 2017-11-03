package operator

import (
	"errors"
	"log"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
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

type EventProcessor interface {
	ProcessPod(podEvent *PodEvent)
	ProcessNode(nodeEvent *NodeEvent)
	ProcessDaemonSet(daemonSetEvent *DaemonSetEvent)
	ProcessDeployment(deploymentEvent *DeploymentEvent)
}

type EventReceiver struct {
	eventsChan chan Event
	processor  EventProcessor
}

func (receiver *EventReceiver) Run() {
	go func() {
		for e := range receiver.eventsChan {
			_, ok := e.(*PodEvent)
			if ok {
				receiver.processor.ProcessPod(e.(*PodEvent))
			}

			_, ok = e.(*NodeEvent)
			if ok {
				receiver.processor.ProcessNode(e.(*NodeEvent))
			}
			// Log unknown event
		}
	}()
}

func (receiver *EventReceiver) Receive(e Event) {
	receiver.eventsChan <- e
}

type HyperpilotOperator struct {
	podInformer       *PodInformer
	deployInformer    *DeploymentInformer
	daemonSetInformer *DaemonSetInformer
	nodeInformer      *NodeInformer

	kclient *kubernetes.Clientset

	mu                 sync.Mutex
	podRegisters       []*EventReceiver
	nodeRegisters      []*EventReceiver
	daemonSetRegisters []*EventReceiver
	deployRegisters    []*EventReceiver

	controllers []BaseController

	nodes map[string]NodeInfo
	pods  map[string]*v1.Pod

	state int
}

// NewHyperpilotOperator creates a new NewHyperpilotOperator
func NewHyperpilotOperator(kclient *kubernetes.Clientset, controllers []EventProcessor) (*HyperpilotOperator, error) {
	baseControllers := []BaseController{}
	resourceEnums := []ResourceEnum{}
	for _, controller := range controllers {
		baseController, ok := controller.(BaseController)
		if !ok {
			return nil, errors.New("Unable to cast controller to BaseController")
		}
		baseControllers = append(baseControllers, baseController)
		resourceEnums = append(resourceEnums, baseController.GetResourceEnum())
	}

	hpc := &HyperpilotOperator{
		podRegisters:       make([]*EventReceiver, 0),
		nodeRegisters:      make([]*EventReceiver, 0),
		daemonSetRegisters: make([]*EventReceiver, 0),
		deployRegisters:    make([]*EventReceiver, 0),
		controllers:        baseControllers,
		kclient:            kclient,
		nodes:              map[string]NodeInfo{},
		pods:               map[string]*v1.Pod{},
		state:              OPERATOR_NOT_RUNNING,
	}

	for i, controller := range controllers {
		hpc.accept(controller, resourceEnums[i])
	}

	return hpc, nil
}

func (c *HyperpilotOperator) ProcessDaemonSet(e *DaemonSetEvent) {

}

func (c *HyperpilotOperator) ProcessDeployment(e *DeploymentEvent) {
	//TODO
	if e.EventType == ADD {
	}

	//TODO
	if e.EventType == DELETE {
	}

	//TODO
	if e.EventType == UPDATE {
	}
}

func (c *HyperpilotOperator) ProcessNode(e *NodeEvent) {
	//TODO
	if e.EventType == ADD {
	}

	//TODO
	if e.EventType == DELETE {
	}

	//TODO
	if e.EventType == UPDATE {
	}
}

func (c *HyperpilotOperator) ProcessPod(e *PodEvent) {
	if e.EventType == DELETE {
		delete(c.pods, e.Cur.Name)
		log.Printf("[ operator ] Delete Pod {%s}", e.Cur.Name)
	}

	// node info is available until pod is in running state
	if e.EventType == UPDATE {
		if e.Old.Status.Phase == "Pending" && e.Cur.Status.Phase == "Running" {
			c.pods[e.Cur.Name] = e.Cur

			log.Printf("[ operator ] Insert NEW Pod {%s}", c.pods[e.Cur.Name])
		}
	}

	for _, podRegister := range c.podRegisters {
		podRegister.Receive(e)
	}
}

// Run starts the process for listening for pod changes and acting upon those changes.
func (c *HyperpilotOperator) Run(stopCh <-chan struct{}) error {
	// Lifecycle:
	c.state = OPERATOR_INITIALIZING

	// 1. Register informers
	c.podInformer = InitPodInformer(c.kclient, c)
	c.nodeInformer = InitNodeInformer(c.kclient, c)

	go c.podInformer.indexInformer.Run(stopCh)
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
		c.pods[p.Name] = &p
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
	c.nodeInformer.onOperatorReady()

	// Wait till we receive a stop signal
	<-stopCh

	return nil
}

func (c *HyperpilotOperator) accept(processor EventProcessor, resourceEnum ResourceEnum) {
	eventReceiver := &EventReceiver{
		eventsChan: make(chan Event, 1000),
		processor:  processor,
	}
	eventReceiver.Run()

	c.mu.Lock()
	defer c.mu.Unlock()

	if resourceEnum.IsRegistered(POD) {
		c.podRegisters = append(c.podRegisters, eventReceiver)
		log.Printf("Contoller {%+v} registered resource POD", processor)
	}

	if resourceEnum.IsRegistered(DEPLOYMENT) {
		c.deployRegisters = append(c.deployRegisters, eventReceiver)
		log.Printf("Contoller {%+v} registered resource DEPLOYMENT", processor)
	}

	if resourceEnum.IsRegistered(DAEMONSET) {
		c.daemonSetRegisters = append(c.daemonSetRegisters, eventReceiver)
		log.Printf("Contoller {%+v} registered resource DAEMONSET", processor)
	}

	if resourceEnum.IsRegistered(NODE) {
		c.nodeRegisters = append(c.nodeRegisters, eventReceiver)
		log.Printf("Contoller {%+v} registered resource NODE", processor)
	}
}
