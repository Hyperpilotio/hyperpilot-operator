package snap

type Microservice struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	ServiceID string `json:"service_id"`
	Namespace string `json:"namespace"`
}

type MetricSource struct {
	APMType string       `json:"APM_type"`
	Port    int          `json:"port"`
	Service Microservice `json:"service"`
}

type AppSLO struct {
	Source MetricSource `json:"source"`
}
