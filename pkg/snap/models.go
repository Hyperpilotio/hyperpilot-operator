package snap

type MicroService struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	ServiceID string `json:"service_id"`
	Namespace string `json:"namespace"`
}
