package snap

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/spf13/viper"
	"gopkg.in/resty.v1"
)

const (
	REGISTERED    = "Registered"
	APPS_API_PATH = "/api/v1/apps?state=Active"
)

type AnalyzerPollHandler interface {
	AppsUpdated(responses []AppResponse)
}

type AnalyzerPoller struct {
	config       *viper.Viper
	handler      AnalyzerPollHandler
	pollInterval time.Duration
	maxInterval  time.Duration
}

type AppResponse struct {
	AppId         string         `json:"app_id"`
	Microservices []MicroService `json:"microservices"`
	Name          string         `json:"name"`
	State         string         `json:"state"`
	Type          string         `json:"type"`
}

type AppResponses struct {
	Data []AppResponse `json:"data"`
}

func NewAnalyzerPoller(config *viper.Viper, handler AnalyzerPollHandler, pollInterval time.Duration, maxInterval time.Duration) *AnalyzerPoller {
	poller := &AnalyzerPoller{
		config:       config,
		handler:      handler,
		pollInterval: pollInterval,
	}

	go poller.run()

	return poller
}

func (analyzerPoller *AnalyzerPoller) run() {
	log.Printf("Starting analyzer poller with interval %s and max %s", analyzerPoller.pollInterval.String(), analyzerPoller.maxInterval.String())
	b := backoff.NewExponentialBackOff()
	b.MaxInterval = analyzerPoller.maxInterval
	b.InitialInterval = analyzerPoller.pollInterval
	b.MaxElapsedTime = 0
	err := backoff.Retry(analyzerPoller.poll, b)
	if err != nil {
		log.Printf("[ AnalyzerPoller ] Polling to Analyzer exited from backoff: " + err.Error())
	}
}

func (analyzerPoller *AnalyzerPoller) poll() error {
	analyzerURL := analyzerPoller.getEndpoint(APPS_API_PATH)
	appResp := AppResponses{}
	resp, err := resty.R().Get(analyzerURL)
	if err != nil {
		log.Printf("[ AnalyzerPoller ] GET all apps from url {%s} error: %s", analyzerURL, err.Error())
		return err
	}

	err = json.Unmarshal(resp.Body(), &appResp)
	if err != nil {
		log.Printf("[ AnalyzerPoller ] Unable unmarshal JSON response from {%s} to AppResponses: %s", analyzerURL, err.Error())
		return err
	}

	analyzerPoller.handler.AppsUpdated(appResp.Data)

	return nil
}

func (analyzerPoller *AnalyzerPoller) getEndpoint(path string) string {
	endpoint := fmt.Sprintf("%s%s%s%d%s",
		"http://", analyzerPoller.config.GetString("SnapTaskController.Analyzer.Address"),
		":", analyzerPoller.config.GetInt("SnapTaskController.Analyzer.Port"), path)
	return endpoint
}
