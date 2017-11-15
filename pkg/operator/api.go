package operator

import (
	"encoding/json"
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

	appGroup := router.Group("/apps")
	{
		appGroup.GET("/:appName/nodes", server.getNodesForApp)
	}

	specGroup := router.Group("/specs")
	{
		specGroup.GET("/namespaces/:namespace/types/:type/:name", server.getSpec)
	}

	router.Group("/actuation")
}

func (server *APIServer) getClusterSpecs(c *gin.Context) {
	var req []SpecRequest
	resp := []SpecResponse{}

	err := c.BindJSON(&req)

	if err != nil {
		log.Printf("Parse Json error: " + err.Error())
		return
	}

	for _, v := range req {

		deploymentResponse := []DeploymentResponse{}
		allDeployment, err := server.getAllDeployment(v.Namespace, v.Deployments)
		if err != nil {
			log.Printf(err.Error())
			return
		}
		for _, d := range allDeployment {
			deploymentResponse = append(deploymentResponse,
				DeploymentResponse{
					Name:     d.Name,
					K8s_spec: d,
				})
		}

		serviceResponse := []ServiceResponse{}
		allService, err := server.getAllService(v.Namespace, v.Services)
		if err != nil {
			log.Printf(err.Error())
			return
		}
		for _, s := range allService {
			serviceResponse = append(serviceResponse,
				ServiceResponse{
					Name:     s.Name,
					K8s_spec: s,
				})
		}

		statefulsetResponse := []StatefulSetResponse{}
		allStateful, err := server.getAllStatefulSet(v.Namespace, v.Statefulsets)
		if err != nil {
			log.Printf(err.Error())
			return
		}
		for _, s := range allStateful {
			statefulsetResponse = append(statefulsetResponse,
				StatefulSetResponse{
					Name:     s.Name,
					K8s_spec: s,
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

func (server *APIServer) getAllDeployment(namespace string, deployments []string) ([]extv1beta1.Deployment, error) {

	var allDeployment = []extv1beta1.Deployment{}

	for _, deploymentName := range deployments {
		option := metav1.ListOptions{
			FieldSelector: "metadata.name=" + deploymentName,
		}
		d, err := server.K8sClient.ExtensionsV1beta1Client.Deployments(namespace).List(option)

		if err != nil {
			log.Printf("List Deployment fail: " + err.Error())
			return nil, err
		}

		//TODO: if not exist, add an non-exist or something
		if len(d.Items) == 0 {
			allDeployment = append(allDeployment, extv1beta1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "Not Exist",
				},
			})
		}

		if len(d.Items) > 1 {
			// Should not be here
			log.Printf("Found multiple deployments {%s} in namespace {%s}", deploymentName, namespace)
		}

		if len(d.Items) != 0 {
			allDeployment = append(allDeployment, d.Items[0])
		}

	}

	return allDeployment, nil
}

func (server *APIServer) getAllService(namespace string, services []string) ([]v1.Service, error) {

	var allService = []v1.Service{}

	for _, serviceName := range services {
		option := metav1.ListOptions{
			FieldSelector: "metadata.name=" + serviceName,
		}

		s, err := server.K8sClient.CoreV1Client.Services(namespace).List(option)

		if err != nil {
			log.Printf("List Service fail: " + err.Error())
			return nil, err
		}

		//TODO: if not exist, add an non-exist or something
		if len(s.Items) == 0 {
			allService = append(allService, v1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name: "Not Exist",
				},
			})
		}

		if len(s.Items) > 1 {
			// Should not be here
			log.Printf("Found multiple deployments {%s} in namespace {%s}", serviceName, namespace)
		}

		if len(s.Items) != 0 {
			allService = append(allService, s.Items[0])
		}

	}

	return allService, nil
}

func (server *APIServer) getAllStatefulSet(namespace string, statefulset []string) ([]appv1beta1.StatefulSet, error) {
	var allStatefulSet = []appv1beta1.StatefulSet{}

	for _, statefulSetName := range statefulset {

		option := metav1.ListOptions{
			FieldSelector: "metadata.name=" + statefulSetName,
		}

		s, err := server.K8sClient.AppsV1beta1Client.StatefulSets(namespace).List(option)

		if err != nil {
			log.Printf("List StatefuleSet fail: " + err.Error())
			return nil, err
		}

		//TODO: if not exist, add an non-exist or something
		if len(s.Items) == 0 {
			allStatefulSet = append(allStatefulSet, appv1beta1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "Not Exist",
				},
			})
		}

		if len(s.Items) > 1 {
			// Should not be here
			log.Printf("Found multiple deployments {%s} in namespace {%s}", allStatefulSet, namespace)
		}

		if len(s.Items) != 0 {
			allStatefulSet = append(allStatefulSet, s.Items[0])
		}
	}

	return allStatefulSet, nil
}

func (server *APIServer) getClusterNodes(c *gin.Context) {

}

func (server *APIServer) getClusterMapping(c *gin.Context) {

}

func (server *APIServer) getNodesForApp(c *gin.Context) {
	c.Param("appName")

	// TODO: Look up k8s objects of this app, and check cluster state where these objects are.

	nodeNames := []string{}
	c.JSON(http.StatusOK, gin.H{
		"error": true,
		"data":  nodeNames,
	})
}

func (server *APIServer) getSpec(c *gin.Context) {
	// e.g: /specs/namespaces/hyperpilot/types/deployment/goddd
	namespace := c.Param("namespace")
	Type := c.Param("type")
	name := c.Param("name")

	option := metav1.ListOptions{
		FieldSelector: "metadata.name=" + name,
	}

	switch {
	case Type == "deployment":
		d, _ := server.K8sClient.ExtensionsV1beta1Client.Deployments(namespace).List(option)

		if len(d.Items) != 1 {
			log.Print("not found")
			return
		}
		b := d.Items[0]
		data, _ := json.Marshal(b)
		log.Print(string(data[:]))

	case Type == "StatefulSet":
		//TODO
	}

}
