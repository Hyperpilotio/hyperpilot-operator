package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/node_spec"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/operator"
	hsnap "github.com/hyperpilotio/hyperpilot-operator/pkg/snap"
	"github.com/spf13/viper"
)

func main() {
	// Set logging output to standard console out
	log.SetOutput(os.Stdout)

	sigs := make(chan os.Signal, 1) // Create channel to receive OS signals
	stop := make(chan struct{})     // Create channel to receive stop signal

	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT) // Register the sigs channel to receieve SIGTERM
	configPath := flag.String("config", "", "The file path to a config file")

	flag.Parse()

	setDefault()
	config, err := ReadConfig(*configPath)
	if err != nil {
		log.Fatalf("Unable to read configure file: %s", err.Error())

	}
	// Create clientset for interacting with the kubernetes cluster
	clientset, err := common.NewK8sClient(config.GetBool("Operator.OutsideCluster"))
	if err != nil {
		log.Fatalf("Unable to create clientset: %s", err.Error())
	}

	log.Printf("[ main ] Starting operator...")

	controllers := loadControllers(config)
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
	hpc.Close()

	// Signal all goroutines in operator to exit
	close(stop)

	log.Printf("Hyperpilot operator exiting")
}

func setDefault() {
	//Default
	viper.SetDefault("SnapTaskController.CreateTaskRetry", 5)
	viper.SetDefault("Operator.OutsideCluster", false)
	viper.SetDefault("SnapTaskController.Analyzer.Enable", true)
	viper.SetDefault("Operator.LoadedControllers",
		[]string{"NodeSpecController", "SnapTaskController"})
	viper.SetDefault("NodeSpecController.CurlPodRestartLimit", 5)
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

	// overwrite by file
	err := viper.ReadInConfig()
	if err != nil {
		return nil, err
	}

	// overwrite by ENV
	if os.Getenv("HP_OUTSIDECLUSTER") == "true" {
		viper.Set("Operator.OutsideCluster", true)
	} else if os.Getenv("HP_OUTSIDECLUSTER") == "false" {
		viper.Set("Operator.OutsideCluster", false)
	}
	if os.Getenv("HP_POLLANALYZERENABLE") == "false" {
		viper.Set("SnapTaskController.Analyzer.Enable", false)
	} else if os.Getenv("HP_POLLANALYZERENABLE") == "true" {
		viper.Set("SnapTaskController.Analyzer.Enable", true)
	}
	if addr := os.Getenv("HP_ANALYZERADDRESS"); addr != "" {
		viper.Set("SnapTaskController.Analyzer.Address", addr)
	}
	if snapYamlUrl := os.Getenv("HP_SNAPYAMLURL"); snapYamlUrl != "" {
		viper.Set("SnapTaskController.SnapDeploymentYamlURL", snapYamlUrl)
	}
	return viper, nil
}

func loadControllers(config *viper.Viper) []operator.EventProcessor {
	controllers := []operator.EventProcessor{}
	controllerSet := common.NewStringSet()

	if ctls := os.Getenv("HP_CONTROLLERS"); ctls != "" {
		controllerSet = common.StringSetFromList(strings.Split(ctls, ","))
	} else {
		controllerSet = common.StringSetFromList(config.GetStringSlice("Operator.LoadedControllers"))
	}

	if controllerSet.IsExist("SnapTaskController") {
		controllers = append(controllers, hsnap.NewSnapTaskController(config))
		log.Printf("[ main ] %s is Loaded", "SnapTaskController")
	}

	if controllerSet.IsExist("SingleSnapController") {
		controllers = append(controllers, hsnap.NewSingleSnapController(config))
		log.Printf("[ main ] %s is Loaded", "SingleSnapController")
	}

	if controllerSet.IsExist("NodeSpecController") {
		controllers = append(controllers, node_spec.NewNodeSpecController(config))
		log.Printf("[ main ] %s is Loaded", "NodeSpecController")
	}
	return controllers
}
