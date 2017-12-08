package api

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
	PodNameList    *[]string              `json:"pod"`
}

type ServiceResponse struct {
	Name        string      `json:"name"`
	ServiceSpec *v1.Service `json:"k8s_spec"`
	PodNameList *[]string   `json:"pod"`
}

type StatefulSetResponse struct {
	Name            string                  `json:"name"`
	StatefulSetSpec *appv1beta1.StatefulSet `json:"k8s_spec"`
	PodNameList     *[]string               `json:"pod"`
}

type MappingResponse struct {
	Namespace    string    `json:"namespace"`
	Deployments  *[]string `json:"deployments,omitempty"`
	Services     *[]string `json:"services,omitempty"`
	Statefulsets *[]string `json:"statefulsets,omitempty"`
}

type Prometheus struct {
	MetricPort int `json:"metricPort"`
}

type MetricRequest struct {
	K8sType    string     `json:"k8sType"`
	Name       string     `json:"name"`
	Namespace  string     `json:"namespace"`
	Prometheus Prometheus `json:"prometheus"`
}

type MetricResponse struct {
	Metric *[]string `json:"metrics"`
}
