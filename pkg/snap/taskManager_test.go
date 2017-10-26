package snap

import (
	"fmt"
	"testing"

	"github.com/intelsdi-x/snap/scheduler/wmap"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	snapURL        = "http://localhost:8181"
	snapAPIVersion = "v1"
)

func TestCreateTask(t *testing.T) {
	Convey("Test snap task manager", t, func() {

		manager, err := NewSnapTaskManager(snapURL, snapAPIVersion)
		So(err, ShouldBeNil)
		So(manager, ShouldNotBeNil)

		// please load these plugin in your snap host
		// pluginList := []string{
		// 	"test-resource/snap-plugin-processor-tag_linux_x86_64",
		// 	"test-resource/snap-plugin-publisher-file_linux_x86_64",
		// 	"test-resource/snap-plugin-collector-cpu_linux_x86_64",
		// }

		// TODO: these code will cause 500 server internal error, don't use for now
		// loadResult := manager.LoadPlugin(pluginList)
		// So(loadResult.Err, ShouldBeNil)
		// So(len(loadResult.LoadedPlugins), ShouldEqual, 3)

		// create collector
		ns := "/intel/procfs"
		metrics := make(map[string]int)
		metrics["/intel/procfs/cpu/*/user_jiffies"] = 7
		config := make(map[string]interface{})
		config["proc_path"] = "/proc"
		collector := NewCollector(ns, metrics, config)

		// create processor
		processorConfig := make(map[string]interface{})
		processorConfig["tags"] = "tag1:value1,tag2:value2"
		processor := NewProcessor("tag", 3, processorConfig)

		// create publisher
		publisherConfig := make(map[string]interface{})
		publisherConfig["file"] = "/tmp/task.log"
		publisher := NewPublisher("file", 2, publisherConfig)

		task, err := NewTask("testTask",
			publisher.JoinProcessor(processor).Join(collector).Join(wmap.NewWorkflowMap()))
		So(err, ShouldBeNil)
		So(task, ShouldNotBeNil)

		id, err := manager.CreateTask(task)
		So(err, ShouldBeNil)
		So(id, ShouldNotBeEmpty)

		result := manager.GetTask(id)
		So(result.Err, ShouldBeNil)
		So(result.Name, ShouldEqual, "testTask")
		fmt.Printf("task status: %v\n", result.State)

		results := manager.GetTasks()
		So(results.Err, ShouldBeNil)
		So(results.Len(), ShouldEqual, 1)
		So(results.ScheduledTasks[0].ID, ShouldEqual, id)

		stopResult := manager.StopTask(id)
		So(stopResult.Err, ShouldBeNil)
		fmt.Println(stopResult.ResponseBodyMessage())
		So(stopResult.ScheduledTaskStopped.ID, ShouldEqual, id)

		startResult := manager.StartTask(id)
		So(startResult.Err, ShouldBeNil)
		fmt.Println(startResult.ResponseBodyMessage())
		So(startResult.ScheduledTaskStarted.ID, ShouldEqual, id)
		manager.StopTask(id)
		removeResult := manager.RemoveTask(id)
		So(removeResult.Err, ShouldBeNil)
		fmt.Println(removeResult.ResponseBodyMessage())
		So(removeResult.ScheduledTaskRemoved.ID, ShouldEqual, id)
	})
}
