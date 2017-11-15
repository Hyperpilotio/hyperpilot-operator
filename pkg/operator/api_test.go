package operator

import (
	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path/filepath"
	"testing"
)

func TestAPI(t *testing.T) {
	kubeConfigLocation := filepath.Join(os.Getenv("KUBECONFIG"))
	config, _ := clientcmd.BuildConfigFromFlags("", kubeConfigLocation)

	kc, _ := kubernetes.NewForConfig(config)

	api_server := NewAPIServer(nil, kc)

	router := gin.Default()

	clusterGroup := router.Group("/cluster")
	{
		clusterGroup.GET("/specs", api_server.getClusterSpecs)
		//clusterGroup.GET("/nodes", server.getClusterNodes)
		//clusterGroup.GET("/mapping/:types", server.getClusterMapping)

	}

	router.Run()
}
