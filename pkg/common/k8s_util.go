package common

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	appv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

func NewK8sClient(runOutsideCluster bool) (*kubernetes.Clientset, error) {
	kubeConfigLocation := ""

	if runOutsideCluster == true {
		if os.Getenv("KUBECONFIG") != "" {
			kubeConfigLocation = filepath.Join(os.Getenv("KUBECONFIG"))
		} else {
			homeDir := os.Getenv("HOME")
			kubeConfigLocation = filepath.Join(homeDir, ".kube", "config")
		}
	}

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigLocation)

	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

func HasDeployment(kclient *kubernetes.Clientset, namespace, deployName string) bool {
	deployClient := kclient.ExtensionsV1beta1Client.Deployments(namespace)

	_, err := deployClient.Get(deployName, metav1.GetOptions{})
	if err != nil {
		log.Printf("[ common ] deployment {%s} is not found ", deployName)
		return false
	}
	log.Printf("[ common ] deployment {%s} is found ", deployName)
	return true
}

func CreateService(kclient *kubernetes.Clientset, namespace, serviceName string, serviceType v1.ServiceType, ports []int32, targetPort []int32) error {
	serviceClient := kclient.CoreV1Client.Services(namespace)
	labels := map[string]string{}
	labels["app"] = "hyperpilot-snap"

	servicePorts := []v1.ServicePort{}
	for i, _ := range ports {
		newPort := v1.ServicePort{
			Port:       ports[i],
			TargetPort: intstr.FromInt(int(targetPort[i])),
		}
		servicePorts = append(servicePorts, newPort)
	}

	internalService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Labels:    labels,
			Namespace: namespace,
		},
		Spec: v1.ServiceSpec{
			Type:      serviceType,
			ClusterIP: "",
			Ports:     servicePorts,
			Selector:  labels,
		},
	}

	if _, err := serviceClient.Create(internalService); err != nil {
		return errors.New("Unable to create service: " + err.Error())
	}

	return nil
}

func HasService(kclient *kubernetes.Clientset, namespace, serviceName string) bool {
	serviceClient := kclient.CoreV1Client.Services(namespace)

	_, err := serviceClient.Get(serviceName, metav1.GetOptions{})
	if err != nil {
		log.Printf("[ common ] service {%s} is not found ", serviceName)
		return false
	}

	log.Printf("[ common ] service {%s} is found ", serviceName)
	return true
}

func CreateDeploymentFromYamlUrl(yamlURL string) (*v1beta1.Deployment, error) {
	resp, err := http.Get(yamlURL)
	if err != nil {
		log.Printf("Http Get from URL {%s} fail: %s", yamlURL, err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	yamlFile, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Read Http response fail: %s", err.Error())
		return nil, err
	}

	var deployment v1beta1.Deployment

	jsonFile, err := yaml.YAMLToJSON(yamlFile)
	if err != nil {
		log.Printf("YAML cannot converted to JSON: %s", err.Error())
		return nil, err
	}

	err = json.Unmarshal(jsonFile, &deployment)
	if err != nil {
		log.Printf("json cannot unmarshal to deployment: %s", err.Error())
		return nil, err
	}

	return &deployment, nil
}

func GetService(K8sClient *kubernetes.Clientset, namespace, serviceName string) (*v1.Service, error) {
	option := metav1.ListOptions{
		FieldSelector: "metadata.name=" + serviceName,
	}
	s, err := K8sClient.CoreV1Client.Services(namespace).List(option)
	if err != nil {
		log.Printf("[ APIServer ] List Service fail: " + err.Error())
		return nil, err
	}

	if len(s.Items) == 0 {
		return nil, nil
	}

	if len(s.Items) > 1 {
		log.Printf("[ APIServer ] Found multiple Services {%s} in namespace {%s}", serviceName, namespace)
	}
	r := s.Items[0]
	return &r, nil
}

func GetDeployment(K8sClient *kubernetes.Clientset, namespace, deploymentName string) (*extv1beta1.Deployment, error) {
	option := metav1.ListOptions{
		FieldSelector: "metadata.name=" + deploymentName,
	}
	d, err := K8sClient.ExtensionsV1beta1Client.Deployments(namespace).List(option)
	if err != nil {
		log.Printf("[ APIServer ] List Deployment fail: " + err.Error())
		return nil, err
	}

	if len(d.Items) == 0 {
		return nil, nil
	}
	if len(d.Items) > 1 {
		log.Printf("[ APIServer ] Found multiple deployments {%s} in namespace {%s}", deploymentName, namespace)
	}
	r := d.Items[0]
	return &r, nil

}

func GetStatefuleSets(K8sClient *kubernetes.Clientset, namespace, statefulSetName string) (*appv1beta1.StatefulSet, error) {
	option := metav1.ListOptions{
		FieldSelector: "metadata.name=" + statefulSetName,
	}
	s, err := K8sClient.AppsV1beta1Client.StatefulSets(namespace).List(option)
	if err != nil {
		log.Printf("[ APIServer ] List StatefulSet fail: " + err.Error())
		return nil, err
	}

	if len(s.Items) == 0 {
		return nil, nil
	}
	if len(s.Items) > 1 {
		log.Printf("[ APIServer ] Found multiple StatefulSets {%s} in namespace {%s}", statefulSetName, namespace)
	}
	r := s.Items[0]
	return &r, nil

}
