package operator

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	appv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type APIServer struct {
	ClusterState *ClusterState
	K8sClient    *kubernetes.Clientset
}

func NewAPIServer(clusterState *ClusterState, k8sClient *kubernetes.Clientset) *APIServer {
	return &APIServer{
		ClusterState: clusterState,
		K8sClient:    k8sClient,
	}
}

func (server *APIServer) Run() {
	router := gin.New()

	// Global middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	clusterGroup := router.Group("/cluster")
	{
		clusterGroup.GET("/specs", server.getClusterSpecs)
		clusterGroup.GET("/nodes", server.getClusterNodes)
		clusterGroup.GET("/mapping/:types", server.getClusterMapping)
	}

	router.Group("/actuation")
}

func (server *APIServer) getClusterSpecs(c *gin.Context) {
	var req []SpecRequest
	resp := []SpecResponse{}
	err := c.BindJSON(&req)
	if err != nil {
		log.Printf("Failed to parse spec request request: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"error": true,
			"cause": "Failed to parse spec request request: " + err.Error(),
		})
		return
	}

	for _, v := range req {
		deploymentResponse := []DeploymentResponse{}
		allDeployment, err := server.getAllDeployment(v.Namespace, v.Deployments)
		if err != nil {
			log.Printf("Unable to get all deployment for namespace %s and deployments %+v: %s", v.Namespace, v.Deployments, err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": true,
				"cause": "List Deployments Failed: " + err.Error(),
			})
			return
		}
		for k, v := range allDeployment {
			deploymentResponse = append(deploymentResponse,
				DeploymentResponse{
					Name:           k,
					DeploymentSpec: v,
				})
		}

		serviceResponse := []ServiceResponse{}
		allService, err := server.getAllService(v.Namespace, v.Services)
		if err != nil {
			log.Printf("Unable to get all deployment for namespace %s and services %+v: %s", v.Namespace, v.Services, err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": true,
				"cause": "List Services Failed: " + err.Error(),
			})
			return
		}
		for k, v := range allService {
			serviceResponse = append(serviceResponse,
				ServiceResponse{
					Name:        k,
					ServiceSpec: v,
				})
		}

		statefulsetResponse := []StatefulSetResponse{}
		allStateful, err := server.getAllStatefulSet(v.Namespace, v.Statefulsets)
		if err != nil {
			log.Printf("Unable to get all deployment for namespace %s and StatefulSet %+v: %s", v.Namespace, v.Statefulsets, err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": true,
				"cause": "List StatefulSets Failed: " + err.Error(),
			})
			return
		}
		for k, v := range allStateful {
			statefulsetResponse = append(statefulsetResponse,
				StatefulSetResponse{
					Name:            k,
					StatefulSetSpec: v,
				})
		}

		resp = append(resp, SpecResponse{
			Namespace:    v.Namespace,
			Deployments:  deploymentResponse,
			Services:     serviceResponse,
			Statefulsets: statefulsetResponse,
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (server *APIServer) getAllDeployment(namespace string, deployments []string) (map[string]*extv1beta1.Deployment, error) {
	allDeployment := map[string]*extv1beta1.Deployment{}
	for _, deploymentName := range deployments {
		option := metav1.ListOptions{
			FieldSelector: "metadata.name=" + deploymentName,
		}
		d, err := server.K8sClient.ExtensionsV1beta1Client.Deployments(namespace).List(option)
		if err != nil {
			log.Printf("List Deployment fail: " + err.Error())
			return nil, err
		}

		if len(d.Items) == 0 {
			allDeployment[deploymentName] = nil
		} else if len(d.Items) > 1 {
			log.Printf("Found multiple deployments {%s} in namespace {%s}", deploymentName, namespace)
		} else {
			r := d.Items[0]
			allDeployment[deploymentName] = &r
		}
	}
	return allDeployment, nil
}

func (server *APIServer) getAllService(namespace string, services []string) (map[string]*v1.Service, error) {
	allService := map[string]*v1.Service{}
	for _, serviceName := range services {
		option := metav1.ListOptions{
			FieldSelector: "metadata.name=" + serviceName,
		}
		s, err := server.K8sClient.CoreV1Client.Services(namespace).List(option)
		if err != nil {
			log.Printf("List Service fail: " + err.Error())
			return nil, err
		}

		if len(s.Items) == 0 {
			allService[serviceName] = nil
		} else if len(s.Items) > 1 {
			log.Printf("Found multiple Services {%s} in namespace {%s}", serviceName, namespace)
		} else {
			r := s.Items[0]
			allService[serviceName] = &r
		}
	}
	return allService, nil
}

func (server *APIServer) getAllStatefulSet(namespace string, statefulset []string) (map[string]*appv1beta1.StatefulSet, error) {
	allStatefulSet := map[string]*appv1beta1.StatefulSet{}
	for _, statefulSetName := range statefulset {
		option := metav1.ListOptions{
			FieldSelector: "metadata.name=" + statefulSetName,
		}
		s, err := server.K8sClient.AppsV1beta1Client.StatefulSets(namespace).List(option)
		if err != nil {
			log.Printf("List StatefulSet fail: " + err.Error())
			return nil, err
		}

		if len(s.Items) == 0 {
			allStatefulSet[statefulSetName] = nil
		} else if len(s.Items) > 1 {
			log.Printf("Found multiple StatefulSets {%s} in namespace {%s}", statefulSetName, namespace)
		} else {
			r := s.Items[0]
			allStatefulSet[statefulSetName] = &r
		}
	}
	return allStatefulSet, nil
}

func (server *APIServer) getClusterNodes(c *gin.Context) {

}

func (server *APIServer) getClusterMapping(c *gin.Context) {

}
