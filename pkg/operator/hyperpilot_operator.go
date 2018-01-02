package operator

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/api"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
)

const (
	hyperpilotOperatorNamespace = common.HyperpilotOperatorNamespace

	// Operator states
	operatorNotRunning              = -1
	operatorInitializing            = 0
	operatorInitializingControllers = 1
	operatorRunning                 = 2
)

type EventProcessor interface {
	ProcessPod(podEvent *common.PodEvent)
	ProcessNode(nodeEvent *common.NodeEvent)
	ProcessDaemonSet(daemonSetEvent *common.DaemonSetEvent)
	ProcessDeployment(deploymentEvent *common.DeploymentEvent)
	ProcessReplicaSet(replicaSetEvent *common.ReplicaSetEvent)
}

type EventReceiver struct {
	eventsChan chan common.Event
	processor  EventProcessor
}

func (receiver *EventReceiver) Run() {
	go func() {
		for e := range receiver.eventsChan {
			_, ok := e.(*common.PodEvent)
			if ok {
				receiver.processor.ProcessPod(e.(*common.PodEvent))
			}

			_, ok = e.(*common.NodeEvent)
			if ok {
				receiver.processor.ProcessNode(e.(*common.NodeEvent))
			}

			_, ok = e.(*common.DaemonSetEvent)
			if ok {
				receiver.processor.ProcessDaemonSet(e.(*common.DaemonSetEvent))
			}

			_, ok = e.(*common.DeploymentEvent)
			if ok {
				receiver.processor.ProcessDeployment(e.(*common.DeploymentEvent))
			}

			_, ok = e.(*common.ReplicaSetEvent)
			if ok {
				receiver.processor.ProcessReplicaSet(e.(*common.ReplicaSetEvent))
			}

			// Log unknown event
		}
	}()
}

func (receiver *EventReceiver) Receive(e common.Event) {
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
		state:              operatorNotRunning,
		config:             config,
	}

	return hpc, nil
}

func (c *HyperpilotOperator) ProcessDaemonSet(e *common.DaemonSetEvent) {}

func (c *HyperpilotOperator) ProcessDeployment(e *common.DeploymentEvent) {}

func (c *HyperpilotOperator) ProcessNode(e *common.NodeEvent) {
	c.clusterState.ProcessNode(e)
	for _, nodeRegister := range c.nodeRegisters {
		nodeRegister.Receive(e)
	}
}

func (c *HyperpilotOperator) ProcessPod(e *common.PodEvent) {
	c.clusterState.ProcessPod(e)
	for _, podRegister := range c.podRegisters {
		podRegister.Receive(e)
	}
}

func (c *HyperpilotOperator) ProcessReplicaSet(e *common.ReplicaSetEvent) {
	c.clusterState.ProcessReplicaSet(e)
	for _, rsRegister := range c.rsRegisters {
		rsRegister.Receive(e)
	}
}

// Run starts the process for listening for pod changes and acting upon those changes.
func (c *HyperpilotOperator) Run(stopCh chan struct{}) error {
	// Lifecycle:
	c.state = operatorInitializing

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
	c.state = operatorInitializingControllers

	controllerWg := &sync.WaitGroup{}
	for _, controller := range c.controllers {
		controllerWg.Add(1)
		// pass controller variable to launch() to avoid reference to the same object.
		go c.launch(controller, controllerWg, stopCh)
	}

	controllerWg.Wait()
	// Use wait group to wait for all controller init to finish

	// 3. Forward events to controllers
	c.state = operatorRunning

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

func (c *HyperpilotOperator) launch(controller BaseController, controllerWg *sync.WaitGroup, stopCh chan struct{}) {
	retryInit := func() error {
		log.Printf("[ operator ] %s run Init(). ", controller)
		return controller.Init(c.clusterState)
	}
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 30 * time.Second
	err := backoff.Retry(retryInit, b)

	if err != nil {
		log.Printf("[ operator ] %s Init() fail after retry. Send signal to shutdown operator: %s", controller, err.Error())
		controllerWg.Done()
		stopCh <- struct{}{}
		return
	}
	c.accept(controller.(EventProcessor), controller.GetResourceEnum())
	controllerWg.Done()
}

func (c *HyperpilotOperator) accept(processor EventProcessor, resourceEnum ResourceEnum) {
	eventReceiver := &EventReceiver{
		eventsChan: make(chan common.Event, 1000),
		processor:  processor,
	}
	eventReceiver.Run()

	c.mu.Lock()
	defer c.mu.Unlock()

	if resourceEnum.IsRegistered(POD) {
		c.podRegisters = append(c.podRegisters, eventReceiver)
		log.Printf("[ operator ] {%+v} registered resource POD", processor)
	}

	if resourceEnum.IsRegistered(DEPLOYMENT) {
		c.deployRegisters = append(c.deployRegisters, eventReceiver)
		log.Printf("[ operator ] {%+v} registered resource DEPLOYMENT", processor)
	}

	if resourceEnum.IsRegistered(DAEMONSET) {
		c.daemonSetRegisters = append(c.daemonSetRegisters, eventReceiver)
		log.Printf("[ operator ] {%+v} registered resource DAEMONSET", processor)
	}

	if resourceEnum.IsRegistered(NODE) {
		c.nodeRegisters = append(c.nodeRegisters, eventReceiver)
		log.Printf("[ operator ] {%+v} registered resource NODE", processor)
	}

	if resourceEnum.IsRegistered(REPLICASET) {
		c.rsRegisters = append(c.rsRegisters, eventReceiver)
		log.Printf("[ operator ] {%+v} registered resource REPLICASET", processor)
	}
}

func (c *HyperpilotOperator) InitApiServer() error {
	err := api.NewAPIServer(c.clusterState, c.kclient, c.config).Run()
	if err != nil {
		return err
	}
	return nil
}

func (c *HyperpilotOperator) Close() {
	for _, controller := range c.controllers {
		controller.Close()
	}
}
