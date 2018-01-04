package common

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	appv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
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
	s, err := K8sClient.CoreV1Client.Services(namespace).Get(serviceName, metav1.GetOptions{})
	if err != nil {
		log.Printf("[ APIServer ] Get Service fail: " + err.Error())
		return nil, err
	}
	return s, nil
}

func ListServices(K8sClient *kubernetes.Clientset, namespace string) ([]v1.Service, error) {
	option := metav1.ListOptions{}
	s, err := K8sClient.CoreV1Client.Services(namespace).List(option)
	if err != nil {
		log.Printf("[ APIServer ] List Service fail: " + err.Error())
		return nil, err
	}
	return s.Items, nil
}

func GetDeployment(K8sClient *kubernetes.Clientset, namespace, deploymentName string) (*extv1beta1.Deployment, error) {
	d, err := K8sClient.ExtensionsV1beta1Client.Deployments(namespace).Get(deploymentName, metav1.GetOptions{})
	if err != nil {
		log.Printf("[ APIServer ] Get Deployment fail: " + err.Error())
		return nil, err
	}
	return d, nil
}

func ListDeployments() ([]*extv1beta1.Deployment, error) {
	return nil, nil
}

func GetStatefuleSets(K8sClient *kubernetes.Clientset, namespace, statefulSetName string) (*appv1beta1.StatefulSet, error) {
	s, err := K8sClient.AppsV1beta1Client.StatefulSets(namespace).Get(statefulSetName, metav1.GetOptions{})
	if err != nil {
		log.Printf("[ APIServer ] Get StatefulSet fail: " + err.Error())
		return nil, err
	}
	return s, nil
}

func ListStatefuleSets() ([]*appv1beta1.StatefulSet, error) {
	return nil, nil
}
