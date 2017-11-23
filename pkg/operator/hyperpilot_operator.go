package operator

import (
	"errors"
	"log"
	"sync"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
)

const (
	HYPERPILOT_OPERATOR_NS = ""

	// Operator states
	OPERATOR_NOT_RUNNING              = -1
	OPERATOR_INITIALIZING             = 0
	OPERATOR_INITIALIZING_CONTROLLERS = 1
	OPERATOR_RUNNING                  = 2
)

type EventProcessor interface {
	ProcessPod(podEvent *PodEvent)
	ProcessNode(nodeEvent *NodeEvent)
	ProcessDaemonSet(daemonSetEvent *DaemonSetEvent)
	ProcessDeployment(deploymentEvent *DeploymentEvent)
	ProcessReplicaSet(replicaSetEvent *ReplicaSetEvent)
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

			_, ok = e.(*DaemonSetEvent)
			if ok {
				receiver.processor.ProcessDaemonSet(e.(*DaemonSetEvent))
			}

			_, ok = e.(*DeploymentEvent)
			if ok {
				receiver.processor.ProcessDeployment(e.(*DeploymentEvent))
			}

			_, ok = e.(*ReplicaSetEvent)
			if ok {
				receiver.processor.ProcessReplicaSet(e.(*ReplicaSetEvent))
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
	rsInformer        *ReplicaSetInformer

	kclient *kubernetes.Clientset

	mu                 sync.Mutex
	podRegisters       []*EventReceiver
	nodeRegisters      []*EventReceiver
	daemonSetRegisters []*EventReceiver
	deployRegisters    []*EventReceiver
	rsRegisters        []*EventReceiver

	controllers []BaseController

	clusterState *common.ClusterState

	state int

	config *viper.Viper
}

// NewHyperpilotOperator creates a new NewHyperpilotOperator
func NewHyperpilotOperator(kclient *kubernetes.Clientset, controllers []EventProcessor, config *viper.Viper) (*HyperpilotOperator, error) {
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
		rsRegisters:        make([]*EventReceiver, 0),
		controllers:        baseControllers,
		kclient:            kclient,
		clusterState:       common.NewClusterState(),
		state:              OPERATOR_NOT_RUNNING,
		config:             config,
	}

	for i, controller := range controllers {
		hpc.accept(controller, resourceEnums[i])
	}

	return hpc, nil
}

func (c *HyperpilotOperator) ProcessDaemonSet(e *DaemonSetEvent) {}

func (c *HyperpilotOperator) ProcessDeployment(e *DeploymentEvent) {}

func (c *HyperpilotOperator) ProcessNode(e *NodeEvent) {}

func (c *HyperpilotOperator) ProcessPod(e *PodEvent) {
	c.clusterState.Lock.Lock()
	if e.EventType == DELETE {
		delete(c.clusterState.Pods, e.Cur.Name)
		log.Printf("[ operator ] Delete Pod {%s}", e.Cur.Name)
	}

	// node info is available until pod is in running state
	if e.EventType == UPDATE {
		if e.Old.Status.Phase == "Pending" && e.Cur.Status.Phase == "Running" {
			c.clusterState.Pods[e.Cur.Name] = e.Cur

			log.Printf("[ operator ] Insert NEW Pod {%s}", e.Cur.Name)
		}
	}
	c.clusterState.Lock.Unlock()

	for _, podRegister := range c.podRegisters {
		podRegister.Receive(e)
	}
}

func (c *HyperpilotOperator) ProcessReplicaSet(e *ReplicaSetEvent) {
	c.clusterState.Lock.Lock()
	if e.EventType == ADD {
		if _, ok := c.clusterState.ReplicaSets[e.Cur.Name]; !ok {
			c.clusterState.ReplicaSets[e.Cur.Name] = e.Cur
			log.Printf("[ operator ] Insert new ReplicaSet {%s}", e.Cur.Name)
		}
	}

	if e.EventType == DELETE {
		delete(c.clusterState.ReplicaSets, e.Cur.Name)
		log.Printf("[ operator ] Delete ReplicaSet {%s},", e.Cur.Name)

	}
	c.clusterState.Lock.Unlock()

	for _, rsRegister := range c.rsRegisters {
		rsRegister.Receive(e)
	}

}

// Run starts the process for listening for pod changes and acting upon those changes.
func (c *HyperpilotOperator) Run(stopCh <-chan struct{}) error {
	// Lifecycle:
	c.state = OPERATOR_INITIALIZING

	// 1. Register informers
	c.podInformer = InitPodInformer(c.kclient, c)
	c.nodeInformer = InitNodeInformer(c.kclient, c)
	c.rsInformer = InitReplicaSetInformer(c.kclient, c)

	go c.podInformer.indexInformer.Run(stopCh)
	go c.nodeInformer.indexInformer.Run(stopCh)
	go c.rsInformer.indexInformer.Run(stopCh)

	// 2. Initialize clusterState state use KubeAPI get pods, .......

	err := c.clusterState.PopulateNodeInfo(c.kclient)
	if err != nil {
		return errors.New("Unable to list nodes from kubernetes: " + err.Error())
	}

	err = c.clusterState.PopulatePods(c.kclient)
	if err != nil {
		return errors.New("Unable to list pods from kubernetes")
	}

	err = c.clusterState.PopulateReplicaSet(c.kclient)
	if err != nil {
		return errors.New("Unable to list ReplicaSet from kubernetes")
	}

	// 3. Initialize controllers
	c.state = OPERATOR_INITIALIZING_CONTROLLERS

	controllerWg := &sync.WaitGroup{}
	for _, controller := range c.controllers {
		controllerWg.Add(1)
		go func() {
			controller.Init(c.clusterState)
			controllerWg.Done()
		}()
	}

	controllerWg.Wait()
	// Use wait group to wait for all controller init to finish

	// 3. Forward events to controllers
	c.state = OPERATOR_RUNNING

	c.podInformer.onOperatorReady()
	c.nodeInformer.onOperatorReady()
	c.rsInformer.onOperatorReady()

	err = c.InitApiServer()
	if err != nil {
		return errors.New("Unable to start API server: " + err.Error())
	}

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

	if resourceEnum.IsRegistered(REPLICASET) {
		c.nodeRegisters = append(c.rsRegisters, eventReceiver)
		log.Printf("Contoller {%+v} registered resource REPLICASET", processor)
	}
}

func (c *HyperpilotOperator) InitApiServer() error {
	err := NewAPIServer(c.clusterState, c.kclient, c.config).Run()
	if err != nil {
		return err
	}
	return nil
}
