package api

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hyperpilotio/hyperpilot-operator/pkg/common"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	appv1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	extv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type APIServer struct {
	ClusterState *common.ClusterState
	K8sClient    *kubernetes.Clientset
	config       *viper.Viper
}

func NewAPIServer(clusterState *common.ClusterState, k8sClient *kubernetes.Clientset, config *viper.Viper) *APIServer {
	return &APIServer{
		ClusterState: clusterState,
		K8sClient:    k8sClient,
		config:       config,
	}
}

func (server *APIServer) Run() error {
	if !server.config.GetBool("APIServer.Debug") {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()

	// Global middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	clusterGroup := router.Group("/cluster")
	{
		clusterGroup.GET("/specs", server.getClusterSpecs)
		clusterGroup.GET("/nodes", server.getClusterNodes)
		clusterGroup.GET("/mapping", server.getClusterMapping)
		clusterGroup.GET("/appmetrics", server.getClusterAppMetrics)
	}
	router.Group("/actuation")

	log.Printf("[ APIServer ] API Server starts")
	err := router.Run(":" + server.config.GetString("APIServer.Port"))
	if err != nil {
		return err
	}
	return nil
}

func (server *APIServer) getClusterSpecs(c *gin.Context) {
	var req []SpecRequest
	resp := []SpecResponse{}
	err := c.BindJSON(&req)
	if err != nil {
		log.Printf("[ APIServer ] Failed to parse spec request request: " + err.Error())
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
			// v is nil if not found
			var list *[]string
			if v != nil {
				v.TypeMeta = metav1.TypeMeta{
					Kind: "Deployment",
				}
				list, err = server.getDeploymenrtPodNameList(v)
				if err != nil {
					log.Printf("[ APIServer ] Can't get pods name of deployment {%s}. Skip : %s", v.Name, err.Error())
					continue
				}
			}
			deploymentResponse = append(deploymentResponse,
				DeploymentResponse{
					Name:           k,
					DeploymentSpec: v,
					PodNameList:    list,
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
			var list *[]string
			if v != nil {
				v.TypeMeta = metav1.TypeMeta{
					Kind: "Service",
				}
				list = server.getServicePodNameList(v)
			}
			serviceResponse = append(serviceResponse,
				ServiceResponse{
					Name:        k,
					ServiceSpec: v,
					PodNameList: list,
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
			// v is nil if not found
			var list *[]string
			if v != nil {
				v.TypeMeta = metav1.TypeMeta{
					Kind: "StatefulSet",
				}
				list = server.getStatefulSetPodNameList(v)
			}
			statefulsetResponse = append(statefulsetResponse,
				StatefulSetResponse{
					Name:            k,
					StatefulSetSpec: v,
					PodNameList:     list,
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

func (server *APIServer) getDeploymenrtPodNameList(deployment *extv1beta1.Deployment) (*[]string, error) {
	hash, err := server.ClusterState.FindReplicaSetHash(deployment.Name)
	if err != nil {
		log.Printf("[ APIServer ] pod-template-hash is not found for deployment {%s}", deployment.Name)
		return nil, err
	}
	pods := server.ClusterState.FindDeploymentPod(deployment.Namespace, deployment.Name, hash)

	podNameList := []string{}
	for _, v := range pods {
		podNameList = append(podNameList, v.Name)
	}
	return &podNameList, nil
}

func (server *APIServer) getStatefulSetPodNameList(statefulset *appv1beta1.StatefulSet) *[]string {
	pods := server.ClusterState.FindStatefulSetPod(statefulset.Namespace, statefulset.Name)
	podNameList := []string{}
	for _, v := range pods {
		podNameList = append(podNameList, v.Name)
	}
	return &podNameList
}

func (server *APIServer) getServicePodNameList(service *v1.Service) *[]string {
	pods := server.ClusterState.FindServicePod(service.Namespace, service)
	podNameList := []string{}
	for _, v := range pods {
		podNameList = append(podNameList, v.Name)
	}
	return &podNameList
}

func (server *APIServer) getAllDeployment(namespace string, deployments []string) (map[string]*extv1beta1.Deployment, error) {
	allDeployment := map[string]*extv1beta1.Deployment{}
	for _, deploymentName := range deployments {
		deploy, err := common.GetDeployment(server.K8sClient, namespace, deploymentName)
		if err != nil {
			log.Printf("[ APIServer ] Get Deployment {%s} in {%s} namespace fail: %s", deploymentName, namespace, err.Error())
			return nil, err
		}
		allDeployment[deploymentName] = deploy
	}
	return allDeployment, nil
}

func (server *APIServer) getAllService(namespace string, services []string) (map[string]*v1.Service, error) {
	allService := map[string]*v1.Service{}
	for _, serviceName := range services {
		service, err := common.GetService(server.K8sClient, namespace, serviceName)
		if err != nil {
			log.Printf("[ APIServer ] Get Service {%s} in {%s} namespace fail: %s", serviceName, namespace, err.Error())
			return nil, err
		}
		allService[serviceName] = service
	}
	return allService, nil
}

func (server *APIServer) getAllStatefulSet(namespace string, statefulset []string) (map[string]*appv1beta1.StatefulSet, error) {
	allStatefulSet := map[string]*appv1beta1.StatefulSet{}
	for _, statefulSetName := range statefulset {
		statefulset, err := common.GetStatefuleSets(server.K8sClient, namespace, statefulSetName)
		if err != nil {
			log.Printf("[ APIServer ] Get Service {%s} in {%s} namespace fail: %s", statefulSetName, namespace, err.Error())
			return nil, err
		}
		allStatefulSet[statefulSetName] = statefulset
	}
	return allStatefulSet, nil
}

func (server *APIServer) getClusterNodes(c *gin.Context) {
	var req []SpecRequest
	err := c.BindJSON(&req)
	if err != nil {
		log.Printf("[ APIServer ] Failed to parse node request: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"error": true,
			"cause": "Failed to parse spec request request: " + err.Error(),
		})
		return
	}

	nodeNameSet := common.NewStringSet()
	for _, v := range req {
		for _, deploy := range v.Deployments {
			hash, err := server.ClusterState.FindReplicaSetHash(deploy)
			if err != nil {
				log.Printf("[ APIServer ] pod-template-hash is not found for deployment {%s}", deploy)
				// if not found, skip this one for now
				continue
			}
			podList := server.ClusterState.FindDeploymentPod(v.Namespace, deploy, hash)
			for _, p := range podList {
				nodeNameSet.Add(p.Spec.NodeName)
			}
		}

		for _, statefulSet := range v.Statefulsets {
			podList := server.ClusterState.FindStatefulSetPod(v.Namespace, statefulSet)
			for _, p := range podList {
				nodeNameSet.Add(p.Spec.NodeName)
			}
		}
	}
	c.JSON(http.StatusOK, nodeNameSet.ToList())
}

func (server *APIServer) getClusterMapping(c *gin.Context) {
	resp := []MappingResponse{}
	req := []string{}
	err := c.BindJSON(&req)
	if err != nil {
		log.Printf("[ APIServer ] Failed to parse mapping request: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"error": true,
			"cause": "Failed to parse mapping request: " + err.Error(),
		})
		return
	}

	namespaceNamesList, err := server.listNamespaces()
	if err != nil {
		log.Printf("[ APIServer ] Unable to get all namespace" + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": true,
			"cause": "List Namespace Failed: " + err.Error(),
		})
		return
	}

	for _, namespaceName := range namespaceNamesList {
		var deploymentResponse *[]string
		var serviceResponse *[]string
		var statefulsetResponse *[]string

		for _, Type := range req {
			switch Type {
			case "deployments":
				deploymentResponse, err = server.listDeployments(namespaceName)
				if err != nil {
					log.Printf("[ APIServer ] Unable to get all deployment for namespace %s: %s", namespaceName, err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": true,
						"cause": "List Deployments Failed: " + err.Error(),
					})
					return
				}
			case "statefulsets":
				statefulsetResponse, err = server.listStatefulSets(namespaceName)
				if err != nil {
					log.Printf("[ APIServer ] Unable to get all StatefulSet for namespace %s: %s", namespaceName, err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": true,
						"cause": "List StatefulSets Failed: " + err.Error(),
					})
					return
				}
			case "services":
				serviceResponse, err = server.listServices(namespaceName)
				if err != nil {
					log.Printf("[ APIServer ] Unable to get all Services for namespace %s: %s", namespaceName, err.Error())
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": true,
						"cause": "List Services Failed: " + err.Error(),
					})
					return
				}
			default:
				log.Print("Unsupport resource type {%s}", Type)
				c.JSON(http.StatusBadRequest, gin.H{
					"error": true,
					"cause": "Unsupported resource type: " + Type,
				})
				return
			}
		}

		resp = append(resp, MappingResponse{
			Namespace:    namespaceName,
			Deployments:  deploymentResponse,
			Services:     serviceResponse,
			Statefulsets: statefulsetResponse,
		})
	}
	c.JSON(http.StatusOK, resp)
}

func (server *APIServer) listNamespaces() ([]string, error) {
	namespaceNames := []string{}
	d, err := server.K8sClient.CoreV1Client.Namespaces().List(metav1.ListOptions{})
	if err != nil {
		log.Printf("[ APIServer ] List Namespaces fail: " + err.Error())
		return nil, err
	}
	for _, namespace := range d.Items {
		namespaceNames = append(namespaceNames, namespace.Name)
	}
	return namespaceNames, nil
}

func (server *APIServer) listDeployments(namespaceName string) (*[]string, error) {
	deploymentNames := []string{}
	d, err := server.K8sClient.ExtensionsV1beta1Client.Deployments(namespaceName).List(metav1.ListOptions{})
	if err != nil {
		log.Printf("[ APIServer ] List Deployments fail: " + err.Error())
		return nil, err
	}

	for _, deploy := range d.Items {
		deploymentNames = append(deploymentNames, deploy.Name)
	}
	return &deploymentNames, nil
}

func (server *APIServer) listStatefulSets(namespaceName string) (*[]string, error) {
	statefulSetNames := []string{}
	d, err := server.K8sClient.AppsV1beta1Client.StatefulSets(namespaceName).List(metav1.ListOptions{})
	if err != nil {
		log.Printf("[ APIServer ] List StatefulSets fail: " + err.Error())
		return nil, err
	}

	for _, stateful := range d.Items {
		statefulSetNames = append(statefulSetNames, stateful.Name)
	}
	return &statefulSetNames, nil
}

func (server *APIServer) listServices(namespaceName string) (*[]string, error) {
	serviceNames := []string{}
	d, err := server.K8sClient.CoreV1Client.Services(namespaceName).List(metav1.ListOptions{})
	if err != nil {
		log.Printf("[ APIServer ] List Services fail: " + err.Error())
		return nil, err
	}
	for _, service := range d.Items {
		serviceNames = append(serviceNames, service.Name)
	}
	return &serviceNames, nil
}

func (server *APIServer) getClusterAppMetrics(c *gin.Context) {
	var req MetricRequest
	err := c.BindJSON(&req)
	if err != nil {
		log.Printf("[ APIServer ] Failed to parse spec request request: " + err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"error": true,
			"cause": "Failed to parse spec request request: " + err.Error(),
		})
		return
	}

	var podList []*v1.Pod

	switch req.K8sType {
	case "deployment":
		hash, err := server.ClusterState.FindReplicaSetHash(req.Name)
		if err != nil {
			log.Printf("[ APIServer ] pod-template-hash is not found for deployment {%s}", req.Name)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": true,
				"cause": "Failed to find pod-template-hash: " + err.Error(),
			})
			return
		}

		podList = server.ClusterState.FindDeploymentPod(req.Namespace, req.Name, hash)
	case "statefulset":
		podList = server.ClusterState.FindStatefulSetPod(req.Namespace, req.Name)
	default:
		log.Printf("[ APIServer ] Kind type {%s} is not supported", req.K8sType)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": true,
			"cause": "Kind type {" + req.K8sType + "} is not supported ",
		})
		return
	}

	if len(podList) == 0 {
		log.Printf("[ APIServer ] Can't find Pod for %s {%s}: %s", req.K8sType, req.Name, err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": true,
			"cause": fmt.Sprintf("Failed to get Pod for %s {%s}: %s \n", req.K8sType, req.Name, err.Error())})
		return
	}

	for _, pod := range podList {
		var url string
		if server.isOutsideCluster() {
			externalIP := server.ClusterState.FindPodRunningNodeInfo(pod.Name).ExternalIP
			url = "http://" + externalIP + ":" + strconv.Itoa(int(req.Prometheus.MetricPort)) + "/metrics"
		} else {
			podIP := pod.Status.PodIP
			url = "http://" + podIP + ":" + strconv.Itoa(int(req.Prometheus.MetricPort)) + "/metrics"
		}
		metricResp, err := getPrometheusMetrics(url)
		if err != nil {
			log.Printf("[ APIServer ] Failed to get Prometheus Metrics from url %s of Pod {%s}, try another Pod : %s ", url, pod.Name, err.Error())
			continue
		}
		c.JSON(http.StatusOK, metricResp)
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{
		"error": true,
		"cause": fmt.Sprintf("Failed to get Prometheus Metrics from all Pods of %s {%s} \n", req.K8sType, req.Name)})
}

func (server *APIServer) isOutsideCluster() bool {
	return server.config.GetBool("Operator.OutsideCluster")
}

func getPrometheusMetrics(url string) (MetricResponse, error) {
	var parser expfmt.TextParser
	var ioReader io.Reader

	resp, err := http.Get(url)
	if err != nil {
		log.Printf("Connect to %s failed :%s", url, err.Error())
		return MetricResponse{}, err
	} else if resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		b := buf.Bytes()
		ioReader = bytes.NewReader(b)
	} else {
		return MetricResponse{}, errors.New(fmt.Sprintf("Unexpected HTTP Status: code: %d Response: %v\n", resp.StatusCode, resp))
	}

	metricFamilies, err := parser.TextToMetricFamilies(ioReader)
	if err != nil {
		log.Printf("Parse metrics from IO reader failed: %s", err.Error())
		return MetricResponse{}, err
	}
	metrics := []Metric{}
	for _, family := range metricFamilies {
		metric := Metric{
			Name: *family.Name,
		}
		tags := []Tag{}
		for _, mm := range family.GetMetric() {
			for _, label := range mm.GetLabel() {
				tags = getTagFromLabel(tags, label)
			}
			switch family.GetType() {
			case dto.MetricType_SUMMARY:
				tag := getTagFromSummary(mm)
				tags = append(tags, tag)
			}
		}
		metric.Tags = &tags
		metrics = append(metrics, metric)
	}
	return MetricResponse{
		Metrics: &metrics,
	}, nil
}

// note: tag key name CAN NOT be summary, conflict with summary
func getTagFromLabel(tags []Tag, label *dto.LabelPair) []Tag {
	for _, tag := range tags {
		if tag.Key == *label.Name {
			*tag.Values = append(*tag.Values, *label.Value)
			return tags
		}
	}
	tags = append(tags, Tag{
		Key:    *label.Name,
		Values: &[]string{*label.Value},
	})
	return tags
}

func getTagFromSummary(metric *dto.Metric) Tag {
	summaryValue := []string{"sum", "count"}
	for _, quantile := range metric.GetSummary().GetQuantile() {
		key := fmt.Sprintf("quantile_%d", int(quantile.GetQuantile()*100))
		summaryValue = append(summaryValue, key)
	}
	return Tag{
		Key:    "summary",
		Values: &summaryValue,
	}
}
