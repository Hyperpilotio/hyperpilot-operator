# Hyperpilot Operator

## Development
### Build Docker Image
`make build-image`

### Build standalone operator
1. `make install_deps`
2. `make build`

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


* HP_CONTROLLERS

  Overwrite Operator.LoadedControllers

  `usage: HP_CONTROLLERS=SnapTaskController,NodeSpecController`
