package operator

type ResourceEnum int

const (
	POD        ResourceEnum = 1 << iota
	DEPLOYMENT ResourceEnum = 2
	DAEMONSET  ResourceEnum = 4
	NODE       ResourceEnum = 8
)

func (this ResourceEnum) IsRegister(flag ResourceEnum) bool {
	return this|flag == this
}

type BaseController interface {
	Init()
	Register(hpc *HyperpilotOpertor, res ResourceEnum)
	Receive(e Event)
	Close()
}

//type BaseController struct {
//}
