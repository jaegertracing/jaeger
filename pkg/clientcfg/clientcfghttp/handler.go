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

package clientcfghttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	p2json "github.com/jaegertracing/jaeger/model/converter/json"
	t2p "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

const mimeTypeApplicationJSON = "application/json"

var errBadRequest = errors.New("bad request")

// HTTPHandlerParams contains parameters that must be passed to NewHTTPHandler.
type HTTPHandlerParams struct {
	ConfigManager  configmanager.ClientConfigManager // required
	MetricsFactory metrics.Factory                   // required

	// BasePath will be used as a prefix for the endpoints, e.g. "/api"
	BasePath string

	// LegacySamplingEndpoint enables returning sampling strategy from "/" endpoint
	// using Thrift 0.9.2 enum codes.
	LegacySamplingEndpoint bool
}

// HTTPHandler implements endpoints for used by Jaeger clients to retrieve client configuration,
// such as sampling and baggage restrictions.
type HTTPHandler struct {
	params  HTTPHandlerParams
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

		// Number of bad responses due to proto conversion
		BadProtoFailures metrics.Counter `metric:"http-server.errors" tags:"status=5xx,source=proto"`

		// Number of failed response writes from http server
		WriteFailures metrics.Counter `metric:"http-server.errors" tags:"status=5xx,source=write"`
	}
}

// NewHTTPHandler creates new HTTPHandler.
func NewHTTPHandler(params HTTPHandlerParams) *HTTPHandler {
	handler := &HTTPHandler{params: params}
	metrics.MustInit(&handler.metrics, params.MetricsFactory, nil)
	return handler
}

// RegisterRoutes registers configuration handlers with Gorilla Router.
func (h *HTTPHandler) RegisterRoutes(router *mux.Router) {
	prefix := h.params.BasePath
	if h.params.LegacySamplingEndpoint {
		router.HandleFunc(
			prefix+"/",
			func(w http.ResponseWriter, r *http.Request) {
				h.serveSamplingHTTP(w, r, h.encodeThriftLegacy)
			},
		).Methods(http.MethodGet)
	}
	router.HandleFunc(
		prefix+"/sampling",
		func(w http.ResponseWriter, r *http.Request) {
			h.serveSamplingHTTP(w, r, h.encodeProto)
		},
	).Methods(http.MethodGet)

	router.HandleFunc(prefix+"/baggageRestrictions", func(w http.ResponseWriter, r *http.Request) {
		h.serveBaggageHTTP(w, r)
	}).Methods(http.MethodGet)
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter, r *http.Request) (string, error) {
	services := r.URL.Query()["service"]
	if len(services) != 1 {
		h.metrics.BadRequest.Inc(1)
		http.Error(w, "'service' parameter must be provided once", http.StatusBadRequest)
		return "", errBadRequest
	}
	return services[0], nil
}

func (h *HTTPHandler) writeJSON(w http.ResponseWriter, json []byte) error {
	w.Header().Add("Content-Type", mimeTypeApplicationJSON)
	if _, err := w.Write(json); err != nil {
		h.metrics.WriteFailures.Inc(1)
		return err
	}
	return nil
}

func (h *HTTPHandler) serveSamplingHTTP(
	w http.ResponseWriter,
	r *http.Request,
	encoder func(strategy *api_v2.SamplingStrategyResponse) ([]byte, error),
) {
	service, err := h.serviceFromRequest(w, r)
	if err != nil {
		return
	}
	resp, err := h.params.ConfigManager.GetSamplingStrategy(r.Context(), service)
	if err != nil {
		h.metrics.CollectorProxyFailures.Inc(1)
		http.Error(w, fmt.Sprintf("collector error: %+v", err), http.StatusInternalServerError)
		return
	}
	jsonBytes, err := encoder(resp)
	if err != nil {
		http.Error(w, "cannot marshall to JSON", http.StatusInternalServerError)
		return
	}
	if err = h.writeJSON(w, jsonBytes); err != nil {
		return
	}
}

func (h *HTTPHandler) encodeThriftLegacy(strategy *api_v2.SamplingStrategyResponse) ([]byte, error) {
	tStrategy, err := t2p.ConvertSamplingResponseFromDomain(strategy)
	if err != nil {
		h.metrics.BadThriftFailures.Inc(1)
		return nil, fmt.Errorf("ConvertSamplingResponseFromDomain failed: %w", err)
	}
	jsonBytes, err := json.Marshal(tStrategy)
	if err != nil {
		h.metrics.BadThriftFailures.Inc(1)
		return nil, err
	}
	jsonBytes = h.encodeThriftEnums092(jsonBytes)
	h.metrics.LegacySamplingRequestSuccess.Inc(1)
	return jsonBytes, nil
}

func (h *HTTPHandler) encodeProto(strategy *api_v2.SamplingStrategyResponse) ([]byte, error) {
	str, err := p2json.SamplingStrategyResponseToJSON(strategy)
	if err != nil {
		h.metrics.BadProtoFailures.Inc(1)
		return nil, fmt.Errorf("SamplingStrategyResponseToJSON failed: %w", err)
	}
	h.metrics.SamplingRequestSuccess.Inc(1)
	return []byte(str), nil
}

func (h *HTTPHandler) serveBaggageHTTP(w http.ResponseWriter, r *http.Request) {
	service, err := h.serviceFromRequest(w, r)
	if err != nil {
		return
	}
	resp, err := h.params.ConfigManager.GetBaggageRestrictions(r.Context(), service)
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

var samplingStrategyTypes = []api_v2.SamplingStrategyType{
	api_v2.SamplingStrategyType_PROBABILISTIC,
	api_v2.SamplingStrategyType_RATE_LIMITING,
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
func (h *HTTPHandler) encodeThriftEnums092(json []byte) []byte {
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
