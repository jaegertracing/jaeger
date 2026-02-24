// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package route

import (
	"context"
	"encoding/json"
	"math"
	"math/rand"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/delay"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/httperr"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
	"github.com/jaegertracing/jaeger/examples/hotrod/services/config"
)

// Server implements Route service
type Server struct {
	hostPort string
	tracer   trace.TracerProvider
	logger   log.Factory
}

// NewServer creates a new route.Server
func NewServer(hostPort string, tracer trace.TracerProvider, logger log.Factory) *Server {
	return &Server{
		hostPort: hostPort,
		tracer:   tracer,
		logger:   logger,
	}
}

// Run starts the Route server
func (s *Server) Run() error {
	mux := s.createServeMux()
	s.logger.Bg().Info("Starting", zap.String("address", "http://"+s.hostPort))
	server := &http.Server{
		Addr:              s.hostPort,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	return server.ListenAndServe()
}

func (s *Server) createServeMux() http.Handler {
	mux := tracing.NewServeMux(false, s.tracer, s.logger)
	mux.Handle("/route", http.HandlerFunc(s.route))
	mux.Handle("/debug/vars", http.HandlerFunc(movedToFrontend))
	mux.Handle("/metrics", http.HandlerFunc(movedToFrontend))
	return mux
}

func movedToFrontend(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "endpoint moved to the frontend service", http.StatusNotFound)
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.logger.For(ctx).Info("HTTP request received", zap.String("method", r.Method), zap.Stringer("url", r.URL))
	if err := r.ParseForm(); httperr.HandleError(w, err, http.StatusBadRequest) {
		s.logger.For(ctx).Error("bad request", zap.Error(err))
		return
	}

	pickup := r.Form.Get("pickup")
	if pickup == "" {
		http.Error(w, "Missing required 'pickup' parameter", http.StatusBadRequest)
		return
	}

	dropoff := r.Form.Get("dropoff")
	if dropoff == "" {
		http.Error(w, "Missing required 'dropoff' parameter", http.StatusBadRequest)
		return
	}

	response := computeRoute(ctx, pickup, dropoff)

	data, err := json.Marshal(response)
	if httperr.HandleError(w, err, http.StatusInternalServerError) {
		s.logger.For(ctx).Error("cannot marshal response", zap.Error(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func computeRoute(ctx context.Context, pickup, dropoff string) *Route {
	start := time.Now()
	defer func() {
		updateCalcStats(ctx, time.Since(start))
	}()

	// Simulate expensive calculation
	delay.Sleep(config.RouteCalcDelay, config.RouteCalcDelayStdDev)

	eta := math.Max(2, rand.NormFloat64()*3+5)
	return &Route{
		Pickup:  pickup,
		Dropoff: dropoff,
		ETA:     time.Duration(eta) * time.Minute,
	}
}
