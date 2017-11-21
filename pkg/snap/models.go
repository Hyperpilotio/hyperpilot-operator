package snap

import (
	"k8s.io/client-go/pkg/api/v1"
	appv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	API_APPS        string = "/api/apps"
	API_K8SSERVICES string = "/api/k8s_services"
)

type ManagementFeature struct {
	Mode              string    `json:"mode"`
	Name              string    `json:"name"`
	RemediationPolicy *[]string `json:"remediation_policy"`
}

type MicroService struct {
	Name      string `json:"name"`
	ServiceID string `json:"service_id"`
	State     string `json:"state"`
}

type SLO struct {
	Metric  string `json:"metric"`
	Summary string `json:"summary"`
	Type    string `json:"type"`
	Unit    string `json:"unit"`
	Value   int    `json:"value"`
}

type AppResponse struct {
	ID                 string              `json:"_id"`
	AppID              string              `json:"app_id"`
	Microservices      []MicroService      `json:"microservices"`
	ManagementFeatures []ManagementFeature `json:"management_features"`
	Name               string              `json:"name"`
	SLO                SLO                 `json:"slo"`
	State              string              `json:"state"`
	Type               string              `json:"type"`
}

type AppResponses struct {
	Data []AppResponse `json:"data"`
}

type ServiceResponse struct {
	ServiceID string `json:"id"`
	Kind      string `json:"kind"`
}

type K8sDeploymentResponse struct {
	ServiceResponse
	Data *extv1beta1.Deployment `json:"data"`
}

type K8sServiceResponse struct {
	ServiceResponse
	Data *v1.Service `json:"data"`
}

type K8sStatefulSetResponse struct {
	ServiceResponse
	Data *appv1beta1.StatefulSet `json:"data"`
}
