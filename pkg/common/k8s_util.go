package common

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
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

func CreateDeploymentFromYamlUrl(yamlURL string) (*v1beta1.Deployment, error) {
	resp, err := http.Get(yamlURL)
	if err != nil {
		log.Printf("%s", err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	yamlFile, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%s", err.Error())
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
		log.Printf("cannot unmarshal data: %s", err.Error())
		return nil, err
	}

	return &deployment, nil
}
