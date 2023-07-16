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

package tracing

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
)

// NewServeMux creates a new TracedServeMux.
func NewServeMux(copyBaggage bool, tracer trace.TracerProvider, logger log.Factory) *TracedServeMux {
	return &TracedServeMux{
		mux:         http.NewServeMux(),
		copyBaggage: copyBaggage,
		tracer:      tracer,
		logger:      logger,
	}
}

// TracedServeMux is a wrapper around http.ServeMux that instruments handlers for tracing.
type TracedServeMux struct {
	mux         *http.ServeMux
	copyBaggage bool
	tracer      trace.TracerProvider
	logger      log.Factory
}

// Handle implements http.ServeMux#Handle, which is used to register new handler.
func (tm *TracedServeMux) Handle(pattern string, handler http.Handler) {
	tm.logger.Bg().Debug("registering traced handler", zap.String("endpoint", pattern))

	middleware := otelhttp.NewHandler(
		otelhttp.WithRouteTag(pattern, traceResponseHandler(handler)),
		pattern,
		otelhttp.WithTracerProvider(tm.tracer))

	tm.mux.Handle(pattern, middleware)
}

// ServeHTTP implements http.ServeMux#ServeHTTP.
func (tm *TracedServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tm.mux.ServeHTTP(w, r)
}

// Returns a handler that generates a traceresponse header.
// https://github.com/w3c/trace-context/blob/main/spec/21-http_response_header_format.md
func traceResponseHandler(handler http.Handler) http.Handler {
	// We use the standard TraceContext propagator, since the formats are identical.
	// But the propagator uses "traceparent" header name, so we inject it into a map
	// `carrier` and then use the result to set the "tracereponse" header.
	var prop propagation.TraceContext
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carrier := make(map[string]string)
		prop.Inject(r.Context(), propagation.MapCarrier(carrier))
		w.Header().Add("traceresponse", carrier["traceparent"])
		handler.ServeHTTP(w, r)
	})
}
