package snap

import (
	"github.com/intelsdi-x/snap/scheduler/wmap"
)

type Collector struct {
	collector *wmap.CollectWorkflowMapNode
}

type Processor struct {
	processor *wmap.ProcessWorkflowMapNode
}

type Publisher struct {
	publisher *wmap.PublishWorkflowMapNode
}

type CollectorOperation interface {
	PutProcessor(processor *Processor) *Collector
	PutPublisher(Publisher *Publisher) *Collector
	Join(workflowMap *wmap.WorkflowMap) *wmap.WorkflowMap
}

type ProcessorOperation interface {
	Put(publisher *Publisher) *Processor
	Join(collector *Collector) *Collector
}

type PublisherOperation interface {
	JoinProcessor(processor *Processor) *Processor
	JoinCollector(collector *Collector) *Collector
}

func NewCollector(namespace string, metrics map[string]int, config map[string]interface{}) *Collector {
	c := wmap.NewCollectWorkflowMapNode()
	for k, v := range config {
		c.AddConfigItem(namespace, k, v)
	}

	for k, v := range metrics {
		c.AddMetric(k, v)
	}
	return &Collector{
		collector: c,
	}
}

func (collector *Collector) PutProcessor(processor *Processor) *Collector {
	collector.collector.Add(processor.processor)
	return collector
}

func (collector *Collector) PutPublisher(publisher *Publisher) *Collector {
	collector.collector.Add(publisher)
	return collector
}

func (collector *Collector) Join(workflowMap *wmap.WorkflowMap) *wmap.WorkflowMap {
	workflowMap.CollectNode = collector.collector
	return workflowMap
}

func NewProcessor(processorName string, processorVersion int, config map[string]interface{}) *Processor {
	p := wmap.NewProcessNode(processorName, processorVersion)
	for k, v := range config {
		p.AddConfigItem(k, v)
	}
	return &Processor{
		processor: p,
	}
}

func (processor *Processor) Put(publisher *Publisher) *Processor {
	processor.processor.Add(publisher.publisher)
	return processor
}

func (processor *Processor) Join(collector *Collector) *Collector {
	collector.collector.Add(processor.processor)
	return collector
}

func NewPublisher(publisherName string, publisherVersion int, config map[string]interface{}) *Publisher {
	p := wmap.NewPublishNode(publisherName, publisherVersion)
	for k, v := range config {
		p.AddConfigItem(k, v)
	}
	return &Publisher{
		publisher: p,
	}
}

func (publisher *Publisher) JoinProcessor(processor *Processor) *Processor {
	processor.processor.Add(publisher.publisher)
	return processor
}

func (publisher *Publisher) JoinCollector(collector *Collector) *Collector {
	collector.collector.Add(publisher.publisher)
	return collector
}
