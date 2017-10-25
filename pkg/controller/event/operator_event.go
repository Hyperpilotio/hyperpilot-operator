package event


type Event interface {
	//TODO: add controller or k8s client in arguments
	Handle()
}
