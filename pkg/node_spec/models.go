package node_spec

type Type struct {
	Type string `json:"type"`
}

type Resource struct {
	CPU    int `json:"cpu"`
	Memory int `json:"memory"`
}

type CustomizedMachineType struct {
	Node     string   `json:"node" bson:"node"`
	Type     string   `json:"type" bson:"type"`
	Resource Resource `json:"resource" bson:"resource"`
}

type PredefinedMachineType struct {
	Node        string `json:"node" bson:"node"`
	Type        string `json:"type" bson:"type"`
	MachineType string `json:"machineType" bson:"machineType"`
}
