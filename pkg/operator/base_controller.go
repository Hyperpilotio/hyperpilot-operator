package operator

type ResourceEnum int

const (
	POD        ResourceEnum = 1 << iota
	DEPLOYMENT ResourceEnum = 2
	DAEMONSET  ResourceEnum = 4
	NODE       ResourceEnum = 8
)

func (this ResourceEnum) IsRegistered(flag ResourceEnum) bool {
	return this|flag == this
}

type BaseController interface {
	Init(clusterState *ClusterState) error
	GetResourceEnum() ResourceEnum
	Close()
}
