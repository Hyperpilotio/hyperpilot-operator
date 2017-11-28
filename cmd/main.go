package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/operator"
	hsnap "github.com/hyperpilotio/hyperpilot-operator/pkg/snap"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// Set logging output to standard console out
	log.SetOutput(os.Stdout)

	sigs := make(chan os.Signal, 1) // Create channel to receive OS signals
	stop := make(chan struct{})     // Create channel to receive stop signal

	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT) // Register the sigs channel to receieve SIGTERM
	configPath := flag.String("config", "", "The file path to a config file")

	flag.Parse()

	config, err := ReadConfig(*configPath)
	if err != nil {
		log.Fatalf("Unable to read configure file: %s", err.Error())

	}
	// Create clientset for interacting with the kubernetes cluster
	clientset, err := newClientSet(config.GetBool("Operator.OutsideCluster"))
	if err != nil {
		log.Fatalf("Unable to create clientset: %s", err.Error())
	}

	log.Printf("Starting operator...")

	controllers := []operator.EventProcessor{}
	controllers = append(controllers, hsnap.NewSnapTaskController(config))
	hpc, err := operator.NewHyperpilotOperator(clientset, controllers, config)
	if err != nil {
		log.Printf("Unable to create hyperpilot operator: " + err.Error())
		return
	}

	go func() {
		err := hpc.Run(stop)
		if err != nil {
			log.Printf("Operator failed to run: " + err.Error())
			close(sigs)
		}
	}()

	// Wait for signal or error from operator
	<-sigs

	// Signal all goroutines in operator to exit
	close(stop)

	log.Printf("Hyperpilot operator exiting")
}

func newClientSet(runOutsideCluster bool) (*kubernetes.Clientset, error) {
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

func ReadConfig(fileConfig string) (*viper.Viper, error) {
	viper := viper.New()
	viper.SetConfigType("json")

	if fileConfig == "" {
		viper.SetConfigName("operator_config")
		viper.AddConfigPath("/etc/operator")
	} else {
		viper.SetConfigFile(fileConfig)
	}

	viper.SetDefault("SnapTaskController.CreateTaskRetry", 5)
	viper.SetDefault("Operator.OutsideCluster", true)

	err := viper.ReadInConfig()
	if err != nil {
		return nil, err
	}
	return viper, nil
}
