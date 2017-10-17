# Hyperpilot Operator

## Usage
`--run-outside-cluster # Uses ~/.kube/config rather than in cluster configuration`

## Development

### Build from source
1. `make install_deps`
2. `make build`
3. `./bin/namespace-rolebinding-operator --run-outside-cluster 1`
