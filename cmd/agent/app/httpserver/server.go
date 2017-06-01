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

package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/uber/jaeger-lib/metrics"

	tSampling "github.com/uber/jaeger/thrift-gen/sampling"
)

const mimeTypeApplicationJSON = "application/json"

var (
	errBadRequest = errors.New("Bad request")
)

// NewHTTPServer creates a new server that hosts an HTTP/JSON endpoint for clients
// to query for sampling strategies and baggage restrictions.
func NewHTTPServer(hostPort string, manager Manager, mFactory metrics.Factory) *http.Server {
	handler := newHTTPHandler(manager, mFactory)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handler.serveSamplingHTTP(w, r, true /* thriftEnums092 */)
	})
	mux.HandleFunc("/sampling", func(w http.ResponseWriter, r *http.Request) {
		handler.serveSamplingHTTP(w, r, false /* thriftEnums092 */)
	})
	mux.HandleFunc("/baggage", func(w http.ResponseWriter, r *http.Request) {
		handler.serveBaggageHTTP(w, r)
	})
	return &http.Server{Addr: hostPort, Handler: mux}
}

func newHTTPHandler(manager Manager, mFactory metrics.Factory) *httpHandler {
	handler := &httpHandler{manager: manager}
	metrics.Init(&handler.metrics, mFactory, nil)
	return handler
}

type httpHandler struct {
	manager Manager
	metrics struct {
		// Number of good sampling requests
		SamplingRequestSuccess metrics.Counter `metric:"http-server.requests" tags:"result=ok,type=sampling"`

		// Number of good sampling requests against the old endpoint / using Thrift 0.9.2 enum codes
		LegacySamplingRequestSuccess metrics.Counter `metric:"http-server.requests" tags:"result=ok,type=sampling-legacy"`

		// Number of good baggage requests
		BaggageRequestSuccess metrics.Counter `metric:"http-server.requests" tags:"result=ok,type=baggage"`

		// Number of bad requests (400s)
		BadRequest metrics.Counter `metric:"http-server.requests" tags:"result=err,status=4xx"`

		// Number of collector proxy failures
		TCollectorProxyFailures metrics.Counter `metric:"http-server.requests" tags:"result=err,status=5xx,type=tcollector-proxy"`

		// Number of bad responses due to malformed thrift
		BadThriftFailures metrics.Counter `metric:"http-server.requests" tags:"result=err,status=5xx,type=thrift"`

		// Number of failed response writes from http server
		WriteFailures metrics.Counter `metric:"http-server.requests" tags:"result=err,status=5xx,type=write"`
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
		h.metrics.TCollectorProxyFailures.Inc(1)
		http.Error(w, fmt.Sprintf("tcollector error: %+v", err), http.StatusInternalServerError)
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
		h.metrics.TCollectorProxyFailures.Inc(1)
		http.Error(w, fmt.Sprintf("tcollector error: %+v", err), http.StatusInternalServerError)
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
