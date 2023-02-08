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

	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
)

// NewServeMux creates a new TracedServeMux.
func NewServeMux(tracer opentracing.Tracer, logger log.Factory) *TracedServeMux {
	return &TracedServeMux{
		mux:    http.NewServeMux(),
		tracer: tracer,
		logger: logger,
	}
}

// TracedServeMux is a wrapper around http.ServeMux that instruments handlers for tracing.
type TracedServeMux struct {
	mux    *http.ServeMux
	tracer opentracing.Tracer
	logger log.Factory
}

// Handle implements http.ServeMux#Handle
func (tm *TracedServeMux) Handle(pattern string, handler http.Handler) {
	tm.logger.Bg().Debug("registering traced handler", zap.String("endpoint", pattern))
	middleware := nethttp.Middleware(
		tm.tracer,
		handler,
		nethttp.OperationNameFunc(func(r *http.Request) string {
			return "HTTP " + r.Method + " " + pattern
		}),
		nethttp.MWSpanObserver(func(span opentracing.Span, r *http.Request) {
			bag := baggage.FromContext(r.Context())
			for _, m := range bag.Members() {
				if b := span.BaggageItem(m.Key()); b == "" {
					span.SetBaggageItem(m.Key(), m.Value())
				}
			}
		}),
	)
	tm.mux.Handle(pattern, otelBaggageExtractor(middleware))
}

// ServeHTTP implements http.ServeMux#ServeHTTP
func (tm *TracedServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tm.mux.ServeHTTP(w, r)
}

func otelBaggageExtractor(next http.Handler) http.Handler {
	propagator := propagation.Baggage{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carrier := propagation.HeaderCarrier(r.Header)
		ctx := propagator.Extract(r.Context(), carrier)
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}
