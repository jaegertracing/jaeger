// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"go.uber.org/zap"

	"github.com/uber/jaeger/thrift-gen/sampling"
)

var (
	errSamplingRateMissing = errors.New("sampling rate is missing")
)

// AgentService is the service used to report traces to the collector.
type AgentService interface {
	GetSamplingRate(service, operation string) (float64, error)
}

type agentService struct {
	url    string
	logger *zap.Logger
}

// NewAgentService returns an instance of AgentService.
func NewAgentService(url string, logger *zap.Logger) AgentService {
	return &agentService{
		url:    url,
		logger: logger,
	}
}

func getSamplingURL(url string) string {
	return url + "/sampling?service=%s"
}

// GetSamplingRate returns the sampling rate for the service-operation from the agent service.
func (s *agentService) GetSamplingRate(service, operation string) (float64, error) {
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
