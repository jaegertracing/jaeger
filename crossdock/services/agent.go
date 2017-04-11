package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"

	"go.uber.org/zap"

	"github.com/uber/jaeger/thrift-gen/sampling"
)

const (
	agentService = "Agent"

	agentURL = "http://test_driver:5778"

	agentCmd = "/go/cmd/jaeger-agent %s &"
)

var (
	errSamplingRateMissing = errors.New("sampling rate is missing")
)

// AgentService is the service used to report traces to the collector.
type AgentService struct {
	url    string
	logger *zap.Logger
}

// InitializeAgent initializes the jaeger agent.
func InitializeAgent(url string, logger *zap.Logger) *AgentService {
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf(agentCmd,
		"-collector.host-port=localhost:14267 -processor.zipkin-compact.server-host-port=test_driver:5775 " +
			"-processor.jaeger-compact.server-host-port=test_driver:6831 -processor.jaeger-binary.server-host-port=test_driver:6832"))
	if err := cmd.Run(); err != nil {
		logger.Fatal("Failed to initialize agent service", zap.Error(err))
	}
	if url == "" {
		url = agentURL
	}
	healthCheck(logger, agentService, agentURL)
	return &AgentService{url: url, logger: logger}
}

func getSamplingURL(url string) string {
	return url + "/sampling?service=%s"
}

// GetSamplingRate returns the sampling rate for the service-operation from the agent service.
func (s AgentService) GetSamplingRate(service, operation string) (float64, error) {
	url := fmt.Sprintf(getSamplingURL(s.url), getTracerServiceName(service))
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	s.logger.Info("Retrieved sampling rates from agent", zap.String("body", string(body)))

	var response sampling.SamplingStrategyResponse
	if err = json.Unmarshal(body, &response); err != nil {
		return 0, err
	}
	return getSamplingRate(operation, &response)
}

func getSamplingRate(operation string, response *sampling.SamplingStrategyResponse) (float64, error) {
	if response.OperationSampling == nil {
		return 0, errSamplingRateMissing
	}
	if len(response.OperationSampling.PerOperationStrategies) == 0 {
		return 0, errSamplingRateMissing
	}
	for _, strategy := range response.OperationSampling.PerOperationStrategies {
		if strategy.Operation == operation {
			return strategy.ProbabilisticSampling.SamplingRate, nil
		}
	}
	return 0, errSamplingRateMissing
}
