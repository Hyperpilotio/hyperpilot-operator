package snap


// Todo: Snap Related

type SnapClient struct {

}

type SnapTask struct {
	Name string
}

func NewSnapClient(name string) *SnapClient {
	return &SnapClient{}
}

func (s *SnapClient) GetAllTasks()([]*SnapTask, error){
	return []*SnapTask{}, nil
}

func (s *SnapClient) CreateSnapTask() *SnapTask{
	return &SnapTask{}
}

func (s *SnapClient) RunTask(task *SnapTask){
	return
}

func (s *SnapClient) DeleteTask(taskName string){
	return
}

