// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/thrift-gen/sampling"
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
