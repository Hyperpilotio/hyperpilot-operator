package operator

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPI(t *testing.T) {
	kubeConfigLocation := filepath.Join(os.Getenv("KUBECONFIG"))
	config, _ := clientcmd.BuildConfigFromFlags("", kubeConfigLocation)

	kc, _ := kubernetes.NewForConfig(config)

	api_server := NewAPIServer(nil, kc, nil)

	router := gin.Default()

	clusterGroup := router.Group("/cluster")
	{
		clusterGroup.GET("/specs", api_server.getClusterSpecs)
		//clusterGroup.GET("/nodes", api_server.getClusterNodes)
		clusterGroup.GET("/mapping", api_server.getClusterMapping)

	}

	router.Run()
}

func TestSet(t *testing.T) {
	set := NewStringSet()

	set.Add("AAAA")
	set.Add("BBBB")
	set.Add("BBBB")
	set.Add("CCCC")

	log.Print(set.ToList())

	set.Remove("BBBB")
	log.Print(set.ToList())
	set.Remove("BBBB")
	log.Print(set.ToList())

	set.Remove("B")
	log.Print(set.ToList())
}

func TestSetAA(t *testing.T) {
	log.Print(strings.Contains("aaaaa", " "))
}

type SomeStruct struct {
	SomeValue *bool `json:"some_value,omitempty"`
}

func TestJSON(G *testing.T) {

	t := new(bool)
	f := new(bool)

	*t = true
	*f = false

	s1, _ := json.Marshal(SomeStruct{nil})
	s2, _ := json.Marshal(SomeStruct{t})
	s3, _ := json.Marshal(SomeStruct{f})

	fmt.Println(string(s1))
	fmt.Println(string(s2))
	fmt.Println(string(s3))

}

type AAA struct {
	Name      string
	ArrayData *[]string `json:"some_value,omitempty"`
}

func TestJSON2(G *testing.T) {

	d := AAA{
		Name:      "aaa",
		ArrayData: &[]string{},
	}

	x := AAA{
		Name:      "aaa",
		ArrayData: nil,
	}

	s1, _ := json.Marshal(d)
	s2, _ := json.Marshal(x)
	fmt.Println(string(s1))
	fmt.Println(string(s2))

}
