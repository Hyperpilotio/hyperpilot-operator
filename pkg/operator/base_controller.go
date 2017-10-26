package operator

type ResourceEnum int

const (
	POD        ResourceEnum = 1 << iota
	DEPLOYMENT ResourceEnum = 2
	DAEMONSET  ResourceEnum = 4
)

func (this ResourceEnum) IsRegister(flag ResourceEnum) bool {
	return this|flag == this
}

type Controller interface {
	Init()
	Register(hpc *HyperpilotOpertor, res ResourceEnum)
	Receive(e Event)
	Close()
}

type BaseController struct {
}
