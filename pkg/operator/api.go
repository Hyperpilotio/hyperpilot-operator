package operator

import (
	"github.com/gin-gonic/gin"
)

type APIServer struct {
	ClusterState *ClusterState
}

func NewAPIServer(clusterState *ClusterState) *APIServer {
	return &APIServer{
		ClusterState: clusterState,
	}
}

func (server *APIServer) Run() {
	router := gin.New()

	// Global middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	appGroup := router.Group("/apps")
	{
		appGroup.GET("/:appName/nodes", server.getNodesForApp)
	}

	actuationGroup := router.Group("/actuation")
	{
	}
}

func (server *APIServer) getNodesForApp(c *gin.Context) {
	appName := c.Param("appName")

	// TODO: Look up k8s objects of this app, and check cluster state where these objects are.

	nodeNames := []string{}
	c.JSON(http.StatusOK, gin.H{
		"error": true,
		"data":  nodeNames,
	})
}
