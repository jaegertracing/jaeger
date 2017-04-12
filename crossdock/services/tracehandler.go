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

	ui "github.com/uber/jaeger/model/json"
)

const (
	servicesParam = "services"
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
	query  QueryService
	agent  AgentService
	logger *zap.Logger
}

// NewTraceHandler returns a TraceHandler that can create traces and verify them
func NewTraceHandler(query QueryService, agent AgentService, logger *zap.Logger) *TraceHandler {
	return &TraceHandler{query: query, agent: agent, logger: logger}
}

// EndToEndTest creates a trace by hitting a client service and validates the trace
func (h *TraceHandler) EndToEndTest(t crossdock.T) {
	operation := generateRandomString()
	request := createTraceRequest(jaeger.SamplerTypeConst, operation, 1)
	service := t.Param(servicesParam)
	h.logger.Info("Starting EndToEnd test", zap.String("service", service))

	if err := h.runTest(service, request, h.createAndRetrieveTraces, validateTracesWithCount); err != nil {
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
	if err := vFunc(request, traces); err != nil {
		return err
	}
	return nil
}

func (h *TraceHandler) createAndRetrieveTraces(service string, request *traceRequest) ([]*ui.Trace, error) {
	if err := createTrace(service, request); err != nil {
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
		h.logger.Info(fmt.Sprintf("Waiting for traces, iteration %d out of 10", i+1))
		traces, err := h.query.GetTraces(getTracerServiceName(service), operation, tags)
		if err == nil && len(traces) > 0 {
			return traces
		}
		h.logger.Info("Could not retrieve trace from query service")
		time.Sleep(time.Second)
	}
	return nil
}

func createTrace(service string, request *traceRequest) error {
	url := fmt.Sprintf("http://%s:8081/create_traces", service)

	b, err := json.Marshal(request)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("retrieved %d status code from client service", resp.StatusCode)
	}
	return nil
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

func createTraceRequest(samplerType string, operation string, count int) *traceRequest {
	return &traceRequest{
		Type:      samplerType,
		Operation: operation,
		Tags:      map[string]string{generateRandomString(): generateRandomString()},
		Count:     count,
	}
}

func generateRandomString() string {
	// A random 8-byte hex
	return fmt.Sprintf("%x", rand.Int63())
}
