package operator

import (
	//"encoding/json"
	//"fmt"
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

	//specGroup := router.Group("/specs")
	//{
	//	specGroup.GET("/namespaces/:namespace/types/:type/:name", api_server.getSpec)
	//}

	clusterGroup := router.Group("/cluster")
	{
		clusterGroup.GET("/specs", api_server.getClusterSpecs)
		//clusterGroup.GET("/nodes", server.getClusterNodes)
		//clusterGroup.GET("/mapping/:types", server.getClusterMapping)

	}

	router.Run()

}

//type App struct {
//	Id    string `json:"id"`
//	Title string `json:"title"`
//}

func TestJson(t *testing.T) {
	//data := []byte(`
	//{
	//   "id": "k34rAT4",
	//   "title": "My Awesome App"
	//}
	//`)

	//var app App
	//json.Unmarshal(data, &app)
	//
	//fmt.Print(string(data[:]))

	//app := App{
	//	Id:    "aaaa",
	//	Title: "bbb",
	//}
	//
	//data, _ := json.Marshal(app)
	//
	//fmt.Print(string(data[:]))

}
