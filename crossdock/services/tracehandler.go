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
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/crossdock/crossdock-go"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-client-go"
	"go.uber.org/zap"

	ui "github.com/jaegertracing/jaeger/model/json"
)

const (
	servicesParam = "services"

	samplerParamKey = "sampler.param"
	samplerTypeKey  = "sampler.type"

	epsilon = 0.00000001
)

var (
	defaultProbabilities = []float64{1.0, 0.001, 0.5}
)

type traceRequest struct {
	Type      string            `json:"type"`
	Operation string            `json:"operation"`
	Tags      map[string]string `json:"tags"`
	Count     int               `json:"count"`
}

type validateFunc func(expected *traceRequest, actual []*ui.Trace) error

type testFunc func(service string, request *traceRequest) ([]*ui.Trace, error)

// TraceHandler handles creating traces and verifying them
type TraceHandler struct {
	query                                 QueryService
	agent                                 AgentService
	logger                                *zap.Logger
	getClientURL                          func(service string) string
	getTags                               func() map[string]string
	createTracesLoopInterval              time.Duration
	getSamplingRateInterval               time.Duration
	clientSamplingStrategyRefreshInterval time.Duration
	getTracesSleepDuration                time.Duration
}

// NewTraceHandler returns a TraceHandler that can create traces and verify them
func NewTraceHandler(query QueryService, agent AgentService, logger *zap.Logger) *TraceHandler {
	return &TraceHandler{
		query:  query,
		agent:  agent,
		logger: logger,
		getClientURL: func(service string) string {
			return fmt.Sprintf("http://%s:8081", service)
		},
		getTags: func() map[string]string {
			return map[string]string{generateRandomString(): generateRandomString()}
		},
		createTracesLoopInterval:              2 * time.Second,
		getSamplingRateInterval:               500 * time.Millisecond,
		clientSamplingStrategyRefreshInterval: 7 * time.Second,
		getTracesSleepDuration:                5 * time.Second,
	}
}

// EndToEndTest creates a trace by hitting a client service and validates the trace
func (h *TraceHandler) EndToEndTest(t crossdock.T) {
	operation := generateRandomString()
	request := h.createTraceRequest(jaeger.SamplerTypeConst, operation, 1)
	service := t.Param(servicesParam)
	h.logger.Info("Starting EndToEnd test", zap.String("service", service))

	if err := h.runTest(service, request, h.createAndRetrieveTraces, validateTracesWithCount); err != nil {
		h.logger.Error(err.Error())
		t.Errorf("Fail: %s", err.Error())
	} else {
		t.Successf("Pass")
	}
}

// AdaptiveSamplingTest creates traces by hitting a client service and validates that the
// sampling probability has changed.
//
// The test creates a stream of traces which gets the adaptive sampler processor to start
// calculating the probability. The test will wait until the sampling rates are calculated
// before creating a large amount of traces with the hopes that at least one trace
// will be sampled with the new sampling probability. The test will make sure the
// new traces were indeed sampled with a calculated probability by checking span tags.
func (h *TraceHandler) AdaptiveSamplingTest(t crossdock.T) {
	operation := generateRandomString()
	request := h.createTraceRequest(jaeger.SamplerTypeRemote, operation, 10)
	service := t.Param(servicesParam)
	h.logger.Info("Starting AdaptiveSampling test", zap.String("service", service))

	if err := h.runTest(service, request, h.adaptiveSamplingTest, validateAdaptiveSamplingTraces); err != nil {
		h.logger.Error(err.Error())
		t.Errorf("Fail: %s", err.Error())
	} else {
		t.Successf("Pass")
	}
}

func (h *TraceHandler) runTest(service string, request *traceRequest, tFunc testFunc, vFunc validateFunc) error {
	traces, err := tFunc(service, request)
	if err != nil {
		return err
	}
	return vFunc(request, traces)
}

func (h *TraceHandler) adaptiveSamplingTest(service string, request *traceRequest) ([]*ui.Trace, error) {
	stop := make(chan struct{})
	go h.createTracesLoop(service, *request, stop)
	defer close(stop)

	var rate float64
	var err error
	for i := 0; i < 20; i++ {
		// Keep checking to see if the sampling rate has been calculated
		h.logger.Info(fmt.Sprintf("Waiting for adaptive sampling probabilities, iteration %d out of 20", i+1))
		rate, err = h.agent.GetSamplingRate(service, request.Operation)
		if err != nil {
			return nil, errors.Wrap(err, "could not retrieve sampling rate from agent")
		}
		if !isDefaultProbability(rate) {
			break
		}
		time.Sleep(h.getSamplingRateInterval)
	}
	if isDefaultProbability(rate) {
		return nil, errors.New("failed to retrieve adaptive sampling rate")
	}

	// Sleep until the clients are guaranteed to get the new sampling rates (they poll the agent every 5 seconds)
	time.Sleep(h.clientSamplingStrategyRefreshInterval)

	request.Count = 500
	request.Tags = map[string]string{"adaptive": "sampling"}
	traces, err := h.createAndRetrieveTraces(service, request)
	if err != nil {
		return nil, err
	}
	return traces, nil
}

func validateAdaptiveSamplingTraces(expected *traceRequest, actual []*ui.Trace) error {
	if err := validateTraces(expected, actual); err != nil {
		return err
	}
	for _, trace := range actual {
		tags := convertTagsIntoMap(trace.Spans[0].Tags)
		samplerParam, ok1 := tags[samplerParamKey]
		samplerType, ok2 := tags[samplerTypeKey]
		if !ok1 || !ok2 {
			return fmt.Errorf("%s and %s tags not found", samplerParamKey, samplerTypeKey)
		}
		probability, err := strconv.ParseFloat(samplerParam, 64)
		if err != nil {
			return fmt.Errorf("%s tag value is not a float: %s", samplerParamKey, samplerParam)
		}
		if samplerType != jaeger.SamplerTypeProbabilistic {
			return fmt.Errorf("%s tag value should be '%s'", samplerTypeKey, jaeger.SamplerTypeProbabilistic)
		}
		if isDefaultProbability(probability) {
			return fmt.Errorf("adaptive sampling probability not used")
		}
	}
	return nil
}

// createTracesLoop creates traces every createTracesLoopInterval.
// The loop can be terminated by closing the stop channel.
func (h *TraceHandler) createTracesLoop(service string, request traceRequest, stop chan struct{}) {
	ticker := time.NewTicker(h.createTracesLoopInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.createTrace(service, &request)
		case <-stop:
			return
		}
	}
}

func (h *TraceHandler) createAndRetrieveTraces(service string, request *traceRequest) ([]*ui.Trace, error) {
	if err := h.createTrace(service, request); err != nil {
		return nil, errors.Wrap(err, "failed to create trace")
	}
	traces := h.getTraces(service, request.Operation, request.Tags)
	if len(traces) == 0 {
		return nil, fmt.Errorf("could not retrieve traces from query service")
	}
	return traces, nil
}

func (h *TraceHandler) getTraces(service, operation string, tags map[string]string) []*ui.Trace {
	// Retry multiple time since SASI indexing takes a couple of seconds
	for i := 0; i < 10; i++ {
		h.logger.Info(fmt.Sprintf("Querying for traces, iteration %d out of 10", i+1))
		traces, err := h.query.GetTraces(getTracerServiceName(service), operation, tags)
		if err == nil && len(traces) > 0 {
			return traces
		}
		h.logger.Info("Could not retrieve trace from query service")
		h.logger.Info(fmt.Sprintf("Waiting %v for traces", h.getTracesSleepDuration))
		time.Sleep(h.getTracesSleepDuration)
	}
	return nil
}

func (h *TraceHandler) createTrace(service string, request *traceRequest) error {
	url := h.getClientURL(service) + "/create_traces"

	// NB. json.Marshal cannot error no matter what traceRequest we give it
	b, _ := json.Marshal(request)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("retrieved %d status code from client service", resp.StatusCode)
	}
	return nil
}

func (h *TraceHandler) createTraceRequest(samplerType string, operation string, count int) *traceRequest {
	return &traceRequest{
		Type:      samplerType,
		Operation: operation,
		Tags:      h.getTags(),
		Count:     count,
	}
}

func validateTracesWithCount(expected *traceRequest, actual []*ui.Trace) error {
	if expected.Count != len(actual) {
		return fmt.Errorf("expected %d trace(s), got %d", expected.Count, len(actual))
	}
	return validateTraces(expected, actual)
}

func validateTraces(expected *traceRequest, actual []*ui.Trace) error {
	for _, trace := range actual {
		if len(trace.Spans) != 1 {
			return fmt.Errorf("expected 1 span, got %d", len(trace.Spans))
		}
		tags := convertTagsIntoMap(trace.Spans[0].Tags)
		if !expectedTagsExist(expected.Tags, tags) {
			return fmt.Errorf("expected tags not found")
		}
	}
	return nil
}

// The real trace has more tags than the tags we sent in, make sure our tags were created
func expectedTagsExist(expected map[string]string, actual map[string]string) bool {
	for k, v := range expected {
		value, ok := actual[k]
		if !ok || value != v {
			return false
		}
	}
	return true
}

func convertTagsIntoMap(tags []ui.KeyValue) map[string]string {
	ret := make(map[string]string)
	for _, tag := range tags {
		if value, ok := tag.Value.(string); ok && tag.Type == ui.StringType {
			ret[tag.Key] = value
		} else if value, ok := tag.Value.(float64); ok && tag.Type == ui.Float64Type {
			ret[tag.Key] = strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	return ret
}

func generateRandomString() string {
	// A random 8-byte hex
	return fmt.Sprintf("%x", rand.Int63())
}

func isDefaultProbability(probability float64) bool {
	for _, p := range defaultProbabilities {
		if floatEquals(p, probability) {
			return true
		}
	}
	return false
}

func floatEquals(a, b float64) bool {
	return (a-b) < epsilon && (b-a) < epsilon
}
