// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"

	p2json "github.com/jaegertracing/jaeger/model/converter/json"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

var errSamplingRateMissing = errors.New("sampling rate is missing")

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
	return url + "/api/sampling?service=%s"
}

// GetSamplingRate returns the sampling rate for the service-operation from the agent service.
func (s *agentService) GetSamplingRate(service, operation string) (float64, error) {
	url := fmt.Sprintf(getSamplingURL(s.url), getTracerServiceName(service))
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	s.logger.Info("Retrieved sampling rates from agent", zap.String("body", string(body)))

	response, err := p2json.SamplingStrategyResponseFromJSON(body)
	if err != nil {
		return 0, err
	}
	return getSamplingRate(operation, response)
}

func getSamplingRate(operation string, response *api_v2.SamplingStrategyResponse) (float64, error) {
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
