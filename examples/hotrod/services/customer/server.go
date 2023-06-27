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

package customer

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/httperr"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// Server implements Customer service
type Server struct {
	hostPort string
	mux      http.ServeMux
	tracer   trace.TracerProvider
	logger   log.Factory
	database *database
}

// NewServer creates a new customer.Server
func NewServer(hostPort string, otelExporter string, metricsFactory metrics.Factory, logger log.Factory) *Server {
	return &Server{
		hostPort: hostPort,
		tracer:   tracing.InitOTEL("customer", otelExporter, metricsFactory, logger),
		logger:   logger,
		database: newDatabase(
			tracing.Init("mysql", otelExporter, metricsFactory, logger),
			logger.With(zap.String("component", "mysql")),
		),
	}
}

// Run starts the Customer server
func (s *Server) Run() error {
	mux := s.createServeMux()
	s.logger.Bg().Info("Starting", zap.String("address", "http://"+s.hostPort))
	return http.ListenAndServe(s.hostPort, otelhttp.NewHandler(mux, "/customer",
		otelhttp.WithTracerProvider(s.tracer)))
}

func (s *Server) createServeMux() http.Handler {
	s.mux.Handle("/customer", otelhttp.WithRouteTag("/customer", http.HandlerFunc(s.customer)))
	return &s.mux
}

func (s *Server) customer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.logger.For(ctx).Info("HTTP request received", zap.String("method", r.Method), zap.Stringer("url", r.URL))
	if err := r.ParseForm(); httperr.HandleError(w, err, http.StatusBadRequest) {
		s.logger.For(ctx).Error("bad request", zap.Error(err))
		return
	}

	customerID := r.Form.Get("customer")
	if customerID == "" {
		http.Error(w, "Missing required 'customer' parameter", http.StatusBadRequest)
		return
	}

	response, err := s.database.Get(ctx, customerID)
	if httperr.HandleError(w, err, http.StatusInternalServerError) {
		s.logger.For(ctx).Error("request failed", zap.Error(err))
		return
	}

	data, err := json.Marshal(response)
	if httperr.HandleError(w, err, http.StatusInternalServerError) {
		s.logger.For(ctx).Error("cannot marshal response", zap.Error(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
