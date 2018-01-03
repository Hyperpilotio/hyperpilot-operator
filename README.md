# Hyperpilot Operator

## Development
### Build Docker Image
`make build-ubuntu-image`

### Build standalone operator
`make install_deps; make build`

## Usage
### Deploy in Kubernetes cluster
1. `kubectl create -f deploy/deploy-serctes.yaml`
2. `kubectl create -f deploy/deploy-operator.yaml`
3. `kubectl create -f deploy/deploy-snap.yaml`

### Run standalone operator (mainly use for developing & testing) 
1. `export KUBECONFIG=/<PATH>/<TO>/kubeconfig`
2. `./bin/hyperpilot-operator [ --conf=<PATH>/<TO>/<CONFIGURE> ]`

## ENV (mainly use for developing & testing)
* HP_OUTSIDECLUSTER

  Overwrite Operator.OutsideCluster.
  
  `usage: HP_OUTSIDECLUSTER=true|false`

* HP_POLLANALYZERENABLE

  Overwrite SnapTaskController.Analyzer.Enable
  
  `usage: HP_POLLANALYZERENABLE=true|false`
  
* HP_ANALYZERADDRESS

  Overwrite SnapTaskController.Analyzer.Address
  
  `usage: HP_ANALYZERADDRESS=127.0.0.1`

* HP_SNAPYAMLURL

  Overwrite SnapTaskController.SnapDeploymentYamlURL
  
  `usage: HP_SNAPYAMLURL=https://s3.us-east-2.amazonaws.com/jimmy-hyperpilot/snap-deployment-sample.yaml`


* HP_CONTROLLERS

  Overwrite Operator.LoadedControllers

  `usage: HP_CONTROLLERS=SingleSnapController,NodeSpecController`
