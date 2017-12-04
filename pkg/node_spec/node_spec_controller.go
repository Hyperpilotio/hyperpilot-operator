package node_spec

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"encoding/json"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/operator"
	"github.com/spf13/viper"
	"gopkg.in/mgo.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
)

type NodeSpecController struct {
	ClusterState *common.ClusterState
	kclient      *kubernetes.Clientset
	config       *viper.Viper
}

func NewNodeSpecController(config *viper.Viper) *NodeSpecController {
	return &NodeSpecController{
		config: config,
	}
}

func (s *NodeSpecController) GetResourceEnum() operator.ResourceEnum {
	return operator.NODE
}

func (s *NodeSpecController) Init(clusterState *common.ClusterState) error {
	s.ClusterState = clusterState
	kclient, err := common.NewK8sClient(s.isOutsideCluster())
	if err != nil {
		log.Printf("[ NodeSpecController ] Create client failure")
		return err
	}
	s.kclient = kclient
	log.Print("[ NodeSpecController ] Init() finished.")
	return nil
}

func (s *NodeSpecController) ProcessNode(e *common.NodeEvent) {
	switch e.EventType {
	case common.ADD:
		go s.getNodeSpec(e.Cur.Name)
		log.Printf("[ NodeSpecController ] Add Node in {%s}", e.Cur.Name)
	case common.DELETE:
		//todo : need to do something when node deletion ??
		log.Printf("[ NodeSpecController ] Delete Node in {%s}", e.Cur.Name)
	}
}

func (s *NodeSpecController) getNodeSpec(nodeName string) {
	podClient := s.kclient.CoreV1Client.Pods("default")
	newPod := s.createCurlPod(nodeName)
	p, err := podClient.Create(newPod)

	if err != nil {
		log.Print("[ NodeSpecController ] Create pod failure: " + err.Error())
		return
	}
	log.Printf("[ NodeSpecController ] Create curl pod {%s}", p.Name)

	for {
		pod, _ := podClient.Get(p.Name, metav1.GetOptions{})
		if len(pod.Status.ContainerStatuses) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}

		if pod.Status.ContainerStatuses[0].State.Terminated == nil {
			log.Printf("[ NodeSpecController ] Curl Pod {%s} still running", pod.Name)
		} else {
			if pod.Status.ContainerStatuses[0].State.Terminated.ExitCode == 0 {
				log.Printf("[ NodeSpecController ] Curl Pod {%s} Completed", pod.Name)
				break
			} else {
				log.Printf("[ NodeSpecController ] Curl Pod {%s} failure", pod.Name)
			}
		}
		time.Sleep(5 * time.Second)
	}

	numTail := int64(1)
	curlResult, err := podClient.GetLogs(newPod.Name, &v1.PodLogOptions{
		TailLines: &numTail,
	}).Do().Raw()
	if err != nil {
		log.Print("[ NodeSpecController ] Get curl pod log fail: " + err.Error())
		return
	}

	parseResult := parseCurlResult(string(curlResult[:]), nodeName)
	err = s.writeToMongo(parseResult)
	if err != nil {
		log.Print("[ NodeSpecController ] Write Machine Type to MongoDB fail" + err.Error())
		return
	}

	err = podClient.Delete(newPod.Name, &metav1.DeleteOptions{})
	if err != nil {
		log.Print("[ NodeSpecController ] Delete curl pod failure: " + err.Error())
		return
	}
}

func (s *NodeSpecController) createCurlPod(nodeName string) *v1.Pod {
	ns := make(map[string]string)
	ns["kubernetes.io/hostname"] = nodeName

	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "curl-" + nodeName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "curl-" + nodeName,
					Image: s.config.GetString("NodeSpecController.Image"),
				},
			},
			NodeSelector:  ns,
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}
	return pod
}

func (s *NodeSpecController) writeToMongo(data []byte) error {
	session, err := mgo.DialWithInfo(&mgo.DialInfo{
		Addrs:    s.config.GetStringSlice("NodeSpecController.MongoDB.Host"),
		Username: s.config.GetString("NodeSpecController.MongoDB.Username"),
		Password: s.config.GetString("NodeSpecController.MongoDB.Password"),
	})
	if err != nil {
		log.Print("[ NodeSpecController ] Connect to Mongo fail" + err.Error())
		return err
	}
	defer session.Close()
	c := session.
		DB(s.config.GetString("NodeSpecController.MongoDB.Database")).
		C(s.config.GetString("NodeSpecController.MongoDB.Collection"))

	var t Type
	var custom CustomizedMachineType
	var predefined PredefinedMachineType

	err = json.Unmarshal(data, &t)
	if err != nil {
		log.Print("[ NodeSpecController ]" + err.Error())
		return err
	}

	if t.Type == "predefined" {
		err := json.Unmarshal(data, &predefined)
		if err != nil {
			log.Printf("[ NodeSpecController ] Unmarshall Json fail: " + err.Error())
			return err
		}
		if err := c.Insert(predefined); err != nil {
			log.Printf("[ NodeSpecController ] Insert into Mongo fail: " + err.Error())
			return err
		}
		log.Printf("[ NodeSpecController ] Insert into Mongo : %s", string(data[:]))
	}

	if t.Type == "customized" {
		err := json.Unmarshal(data, &custom)
		if err != nil {
			log.Printf("[ NodeSpecController ] Unmarshall Json fail: " + err.Error())
			return err
		}
		if err := c.Insert(custom); err != nil {
			log.Printf("[ NodeSpecController ] Insert into Mongo fail: " + err.Error())
			return err
		}
		log.Printf("[ NodeSpecController ] Insert into Mongo : %s", string(data[:]))
	}

	return nil
}

func parseCurlResult(curlResult, nodeName string) []byte {
	s1 := strings.Split(curlResult, "/")
	machineType := s1[len(s1)-1]

	if strings.HasPrefix(machineType, "custom") {
		resource := strings.Split(machineType, "-")
		cpu, err := strconv.Atoi(resource[1])
		if err != nil {
			log.Printf(err.Error())
			return nil
		}
		memory, _ := strconv.Atoi(resource[2])
		if err != nil {
			log.Printf(err.Error())
			return nil
		}

		customizedType := CustomizedMachineType{
			Node: nodeName,
			Type: "customized",
			Resource: Resource{
				CPU:    cpu,
				Memory: memory,
			},
		}
		b, err := json.Marshal(customizedType)
		if err != nil {
			log.Printf(err.Error())
			return nil
		}
		return b
	} else {
		// todo: Better check?
		predefinedType := PredefinedMachineType{
			Node:        nodeName,
			Type:        "predefined",
			MachineType: machineType,
		}
		b, err := json.Marshal(predefinedType)
		if err != nil {
			log.Printf(err.Error())
			return nil
		}
		return b
	}

	return nil
}

func (s *NodeSpecController) ProcessPod(e *common.PodEvent) {}

func (s *NodeSpecController) ProcessDeployment(e *common.DeploymentEvent) {}

func (s *NodeSpecController) ProcessDaemonSet(e *common.DaemonSetEvent) {}

func (s *NodeSpecController) ProcessReplicaSet(e *common.ReplicaSetEvent) {}

func (s *NodeSpecController) String() string {
	return fmt.Sprintf("NodeSpecController")
}

func (s *NodeSpecController) isOutsideCluster() bool {
	return s.config.GetBool("Operator.OutsideCluster")
}
