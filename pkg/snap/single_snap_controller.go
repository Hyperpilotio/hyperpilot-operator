package snap

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/operator"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const HYPERPILOT_SNAP_NS = "hyperpilot"
const HYPERPILOT_SNAP_DEPLOYMENT_NAME = "hyperpilot-snap"

type SingleSnapController struct {
	ServiceList      []string
	SnapNode         *SnapNode
	DeletingSnapNode *SnapNode
	config           *viper.Viper
	ClusterState     *common.ClusterState
	K8sClient        *kubernetes.Clientset
	analyzerPoller   *AnalyzerPoller
}

func NewSingleSnapController(config *viper.Viper) *SingleSnapController {
	return &SingleSnapController{
		ServiceList: config.GetStringSlice("SnapTaskController.ServiceList"),
		SnapNode:    nil,
		config:      config,
	}
}

func (s *SingleSnapController) GetResourceEnum() operator.ResourceEnum {
	return operator.POD
}

func (s *SingleSnapController) Init(clusterState *common.ClusterState) error {
	s.ClusterState = clusterState
	// get k8s client
	s.ClusterState = clusterState
	kclient, err := common.NewK8sClient(s.isOutsideCluster())
	s.K8sClient = kclient
	if err != nil {
		log.Printf("[ SingleSnapController ] Create client failure")
		return err
	}
	deployClient := kclient.ExtensionsV1beta1Client.Deployments(HYPERPILOT_SNAP_NS)

	// build snap spec
	deployment := s.makeSnapDeployment()

	// create snap deployment
	_, err = deployClient.Create(deployment)
	if err != nil {
		log.Printf("[ SingleSnapController ] Create Snap Deployment fail: %s ", err.Error())
		return err
	}

	// create SnapNode util deployment is available
	for {
		d, err := deployClient.Get(deployment.Name, metav1.GetOptions{})
		if err != nil {
			log.Printf("[ SingleSnapController ] Check Snap Deployment status fail: %s ", err.Error())
			return err
		}

		for _, cond := range d.Status.Conditions {
			if cond.Type == v1beta1.DeploymentAvailable && cond.Status == v1.ConditionTrue {
				if err := s.createSnapNode(); err != nil {
					log.Printf("[ SingleSnapController ] Create SnapNode fail: %s ", err.Error())
				}
				log.Print("[ SingleSnapController ] Init() finished, create SnapNode")
				return nil
			}
		}
		log.Print("[ SingleSnapController ] wait create snap node")
		time.Sleep(5 * time.Second)
	}

	if s.config.GetBool("SnapTaskController.Analyzer.Enable") {
		log.Printf("[ SnapTaskController ] Poll Analyzer is enabled")
		s.analyzerPoller = NewAnalyzerPoller(s.config, s)
		go s.analyzerPoller.pollingAnalyzer()
		//go s.pollingAnalyzer()
	}

	return nil
}

func int32Ptr(i int32) *int32 { return &i }

func bool2Ptr(b bool) *bool { return &b }

func (s *SingleSnapController) makeSnapDeploymentSpec() *v1beta1.DeploymentSpec {

	var conPort v1.ContainerPort
	if s.isOutsideCluster() {
		conPort = v1.ContainerPort{
			ContainerPort: 8181,
			HostPort:      8181,
		}
	} else {
		conPort = v1.ContainerPort{
			ContainerPort: 8181,
		}
	}

	return &v1beta1.DeploymentSpec{
		Replicas: int32Ptr(1),
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app":     "snap",
					"version": "latest",
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Image: "hyperpilot/snap:xenial",
						Name:  "snap",
						Command: []string{
							"/usr/local/bin/run.sh",
						},
						Args: []string{
							"https://s3.us-east-2.amazonaws.com/jimmy-hyperpilot/init-resource-worker.json",
						},
						ImagePullPolicy: v1.PullAlways,
						Ports: []v1.ContainerPort{
							conPort,
						},
						SecurityContext: &v1.SecurityContext{
							Privileged: bool2Ptr(true),
						},
						Env: []v1.EnvVar{
							{
								Name: "NODE_NAME",
								ValueFrom: &v1.EnvVarSource{
									FieldRef: &v1.ObjectFieldSelector{
										FieldPath: "spec.nodeName",
									},
								},
							},
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      "var-run",
								MountPath: "/var/run",
							},
							{
								Name:      "var-log",
								MountPath: "/var/log",
							},
							{
								Name:      "cgroup",
								MountPath: "/sys/fs/cgroup",
							},
							{
								Name:      "var-lib-docker",
								MountPath: "/var/lib/docker",
							},
							{
								Name:      "usr-bin-docker",
								MountPath: "/usr/local/bin/docker",
							},
							{
								Name:      "proc",
								MountPath: "/proc_host",
							},
						},
					},
				},
				Volumes: []v1.Volume{
					{
						Name: "cgroup",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/sys/fs/cgroup",
							},
						},
					},
					{
						Name: "var-lib-docker",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/var/lib/docker/",
							},
						},
					},
					{
						Name: "var-log",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/var/log",
							},
						},
					},
					{
						Name: "var-run",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/var/run",
							},
						},
					},
					{
						Name: "usr-bin-docker",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/usr/bin/docker",
							},
						},
					},
					{
						Name: "proc",
						VolumeSource: v1.VolumeSource{
							HostPath: &v1.HostPathVolumeSource{
								Path: "/proc",
							},
						},
					},
				},
			},
		},
	}
}

func (s *SingleSnapController) makeSnapDeployment() *v1beta1.Deployment {
	spec := s.makeSnapDeploymentSpec()

	return &v1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HYPERPILOT_SNAP_DEPLOYMENT_NAME,
			Namespace: HYPERPILOT_SNAP_NS,
		},
		Spec: *spec,
	}
}

func (s *SingleSnapController) createSnapNode() error {
	replicaClient := s.K8sClient.ExtensionsV1beta1Client.ReplicaSets(HYPERPILOT_SNAP_NS)
	replicaList, err := replicaClient.List(metav1.ListOptions{})
	if err != nil {
		log.Printf("[ SingleSnapController ] List replicaSet fail: %s", err.Error())
		return err
	}

	hash := ""
	for _, replicaSet := range replicaList.Items {
		for _, ref := range replicaSet.OwnerReferences {
			if ref.Kind == "Deployment" && ref.Name == HYPERPILOT_SNAP_DEPLOYMENT_NAME {
				hash = replicaSet.ObjectMeta.Labels["pod-template-hash"]
			}
		}
	}
	if hash == "" {
		return errors.New(fmt.Sprintf("pod-template-hash label is not found for Deployment {%s}", HYPERPILOT_SNAP_DEPLOYMENT_NAME))
	}

	podClient := s.K8sClient.CoreV1Client.Pods(HYPERPILOT_SNAP_NS)
	pods, err := podClient.List(metav1.ListOptions{
		LabelSelector: "pod-template-hash=" + hash,
	})

	if err != nil {
		log.Printf("[ SingleSnapController ] List Pod with Label pod-template-hash=%s fail: %s", hash, err.Error())
		return err
	}

	if len(pods.Items) == 0 {
		return errors.New(fmt.Sprintf("[ SingleSnapController ] can't find Pod with Label pod-template-hash=%s", hash))
	}
	nodeName := pods.Items[0].Spec.NodeName
	s.SnapNode = NewSnapNode(nodeName, s.ClusterState.Nodes[nodeName].ExternalIP, &s.ServiceList, s.config)
	if err := s.SnapNode.initSingleSnap(s.isOutsideCluster(), &pods.Items[0], s.ClusterState); err != nil {
		log.Printf("[ SingleSnapController ] SnapNode Init fail : %s", err.Error())
		return err
	}
	return nil
}

func (s *SingleSnapController) isOutsideCluster() bool {
	return s.config.GetBool("Operator.OutsideCluster")
}

func (s *SingleSnapController) String() string {
	return fmt.Sprintf("SingleSnapController")
}

func (s *SingleSnapController) ProcessPod(e *common.PodEvent) {
	switch e.EventType {
	case common.DELETE:
		if e.Cur.Status.Phase != "Running" {
			return
		}
		if isHyperPilotSnapPod(e.Cur) {
			log.Printf("[ SingleSnapController ] Delete SnapNode in {%s}", s.DeletingSnapNode.NodeId)
			s.DeletingSnapNode.Exit()
		}
		if s.SnapNode.isServicePod(e.Cur) {
			s.SnapNode.PodEvents <- e
		}
	case common.ADD, common.UPDATE:
		if e.Cur.Status.Phase == "Running" && (e.Old == nil || e.Old.Status.Phase == "Pending") {
			nodeName := e.Cur.Spec.NodeName
			if isHyperPilotSnapPod(e.Cur) {
				if s.SnapNode != nil {
					s.DeletingSnapNode = s.SnapNode
				}
				newNode := NewSnapNode(nodeName, s.ClusterState.Nodes[nodeName].ExternalIP, &s.ServiceList, s.config)
				s.SnapNode = newNode
				go func() {
					if err := s.SnapNode.initSingleSnap(s.isOutsideCluster(), e.Cur, s.ClusterState); err != nil {
						log.Printf("[ SingleSnapController ] Create new SnapNode ")
					}
				}()
			}
			if s.SnapNode.isServicePod(e.Cur) {
				s.SnapNode.PodEvents <- e
			}
		}
	}
}

func isHyperPilotSnapPod(pod *v1.Pod) bool {
	if strings.HasPrefix(pod.Name, HYPERPILOT_SNAP_DEPLOYMENT_NAME) {
		return true
	}
	return false
}

func (s *SingleSnapController) ProcessDeployment(e *common.DeploymentEvent) {}

func (s *SingleSnapController) ProcessDaemonSet(e *common.DaemonSetEvent) {}

func (s *SingleSnapController) ProcessNode(e *common.NodeEvent) {}

func (s *SingleSnapController) ProcessReplicaSet(e *common.ReplicaSetEvent) {}

func (s *SingleSnapController) Close() {
	deletePolicy := metav1.DeletePropagationForeground
	deploymentsClient := s.K8sClient.ExtensionsV1beta1Client.Deployments(HYPERPILOT_SNAP_NS)
	if err := deploymentsClient.Delete(HYPERPILOT_SNAP_DEPLOYMENT_NAME, &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		panic(err)
	}
	log.Printf("[ SingleSnapController ] Delete deployment {%s}", HYPERPILOT_SNAP_DEPLOYMENT_NAME)
	//todo: wait until finish
}
