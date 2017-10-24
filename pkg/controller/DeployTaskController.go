package controller


import 	(
	"github.com/satori/go.uuid"

)


type DeployTaskController struct{
	// controller id
	uuid uuid.UUID

	// match list
	matchList []Match

	// receive event from operator
	TodoEvents <-chan event
}
