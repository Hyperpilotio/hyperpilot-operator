package operator

import (
	"k8s.io/client-go/pkg/api/v1"
	appv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type SpecRequest struct {
	Namespace    string   `json:"namespace"`
	Deployments  []string `json:"deployments"`
	Services     []string `json:"services"`
	Statefulsets []string `json:"statefulsets"`
}

type SpecResponse struct {
	Namespace    string                `json:"namespace"`
	Deployments  []DeploymentResponse  `json:"deployments"`
	Services     []ServiceResponse     `json:"services"`
	Statefulsets []StatefulSetResponse `json:"statefulsets"`
}

type DeploymentResponse struct {
	Name           string                 `json:"name"`
	DeploymentSpec *extv1beta1.Deployment `json:"k8s_spec"`
}

type ServiceResponse struct {
	Name        string      `json:"name"`
	ServiceSpec *v1.Service `json:"k8s_spec"`
}

type StatefulSetResponse struct {
	Name            string                  `json:"name"`
	StatefulSetSpec *appv1beta1.StatefulSet `json:"k8s_spec"`
}
