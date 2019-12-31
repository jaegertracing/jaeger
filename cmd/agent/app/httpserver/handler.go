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

package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	tSampling "github.com/jaegertracing/jaeger/thrift-gen/sampling"
)

const mimeTypeApplicationJSON = "application/json"

var (
	errBadRequest = errors.New("bad request")
)

func newHTTPHandler(manager configmanager.ClientConfigManager, mFactory metrics.Factory) *httpHandler {
	handler := &httpHandler{manager: manager}
	metrics.Init(&handler.metrics, mFactory, nil)
	return handler
}

type httpHandler struct {
	manager configmanager.ClientConfigManager
	metrics struct {
		// Number of good sampling requests
		SamplingRequestSuccess metrics.Counter `metric:"http-server.requests" tags:"type=sampling"`

		// Number of good sampling requests against the old endpoint / using Thrift 0.9.2 enum codes
		LegacySamplingRequestSuccess metrics.Counter `metric:"http-server.requests" tags:"type=sampling-legacy"`

		// Number of good baggage requests
		BaggageRequestSuccess metrics.Counter `metric:"http-server.requests" tags:"type=baggage"`

		// Number of bad requests (400s)
		BadRequest metrics.Counter `metric:"http-server.errors" tags:"status=4xx,source=all"`

		// Number of collector proxy failures
		CollectorProxyFailures metrics.Counter `metric:"http-server.errors" tags:"status=5xx,source=collector-proxy"`

		// Number of bad responses due to malformed thrift
		BadThriftFailures metrics.Counter `metric:"http-server.errors" tags:"status=5xx,source=thrift"`

		// Number of failed response writes from http server
		WriteFailures metrics.Counter `metric:"http-server.errors" tags:"status=5xx,source=write"`
	}
}

func (h *httpHandler) serviceFromRequest(w http.ResponseWriter, r *http.Request) (string, error) {
	services := r.URL.Query()["service"]
	if len(services) != 1 {
		h.metrics.BadRequest.Inc(1)
		http.Error(w, "'service' parameter must be provided once", http.StatusBadRequest)
		return "", errBadRequest
	}
	return services[0], nil
}

func (h *httpHandler) writeJSON(w http.ResponseWriter, json []byte) error {
	w.Header().Add("Content-Type", mimeTypeApplicationJSON)
	if _, err := w.Write(json); err != nil {
		h.metrics.WriteFailures.Inc(1)
		return err
	}
	return nil
}

func (h *httpHandler) serveSamplingHTTP(w http.ResponseWriter, r *http.Request, thriftEnums092 bool) {
	service, err := h.serviceFromRequest(w, r)
	if err != nil {
		return
	}
	resp, err := h.manager.GetSamplingStrategy(service)
	if err != nil {
		h.metrics.CollectorProxyFailures.Inc(1)
		http.Error(w, fmt.Sprintf("collector error: %+v", err), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		h.metrics.BadThriftFailures.Inc(1)
		http.Error(w, "Cannot marshall Thrift to JSON", http.StatusInternalServerError)
		return
	}
	if thriftEnums092 {
		jsonBytes = h.encodeThriftEnums092(jsonBytes)
	}
	if err = h.writeJSON(w, jsonBytes); err != nil {
		return
	}
	if thriftEnums092 {
		h.metrics.LegacySamplingRequestSuccess.Inc(1)
	} else {
		h.metrics.SamplingRequestSuccess.Inc(1)
	}
}

func (h *httpHandler) serveBaggageHTTP(w http.ResponseWriter, r *http.Request) {
	service, err := h.serviceFromRequest(w, r)
	if err != nil {
		return
	}
	resp, err := h.manager.GetBaggageRestrictions(service)
	if err != nil {
		h.metrics.CollectorProxyFailures.Inc(1)
		http.Error(w, fmt.Sprintf("collector error: %+v", err), http.StatusInternalServerError)
		return
	}
	// NB. it's literally impossible for this Marshal to fail
	jsonBytes, _ := json.Marshal(resp)
	if err = h.writeJSON(w, jsonBytes); err != nil {
		return
	}
	h.metrics.BaggageRequestSuccess.Inc(1)
}

var samplingStrategyTypes = []tSampling.SamplingStrategyType{
	tSampling.SamplingStrategyType_PROBABILISTIC,
	tSampling.SamplingStrategyType_RATE_LIMITING,
}

// Replace string enum values produced from Thrift 0.9.3 generated classes
// with integer codes produced from Thrift 0.9.2 generated classes.
//
// For example:
//
// Thrift 0.9.2 classes generate this JSON:
// {"strategyType":0,"probabilisticSampling":{"samplingRate":0.5},"rateLimitingSampling":null,"operationSampling":null}
//
// Thrift 0.9.3 classes generate this JSON:
// {"strategyType":"PROBABILISTIC","probabilisticSampling":{"samplingRate":0.5}}
func (h *httpHandler) encodeThriftEnums092(json []byte) []byte {
	str := string(json)
	for _, strategyType := range samplingStrategyTypes {
		str = strings.Replace(
			str,
			fmt.Sprintf(`"strategyType":"%s"`, strategyType.String()),
			fmt.Sprintf(`"strategyType":%d`, strategyType),
			1,
		)
	}
	return []byte(str)
}
