package snap

const (
	apiApps string = "/api/v1/apps"
)

type MicroService struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	ServiceID string `json:"service_id"`
	Namespace string `json:"namespace"`
}

type AppResponse struct {
	ID            string         `json:"_id"`
	AppID         string         `json:"app_id"`
	Microservices []MicroService `json:"microservices"`
	Name          string         `json:"name"`
	State         string         `json:"state"`
	Type          string         `json:"type"`
}

type AppResponses struct {
	Data []AppResponse `json:"data"`
}
