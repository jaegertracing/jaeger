// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package frontend

import (
	"embed"
	"encoding/json"
	"expvar"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/httperr"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/log"
	"github.com/jaegertracing/jaeger/examples/hotrod/pkg/tracing"
	"github.com/jaegertracing/jaeger/pkg/httpfs"
)

//go:embed web_assets/*
var assetFS embed.FS

// Server implements jaeger-demo-frontend service
type Server struct {
	hostPort string
	tracer   trace.TracerProvider
	logger   log.Factory
	bestETA  *bestETA
	assetFS  http.FileSystem
	basepath string
	jaegerUI string
}

// ConfigOptions used to make sure service clients
// can find correct server ports
type ConfigOptions struct {
	FrontendHostPort string
	DriverHostPort   string
	CustomerHostPort string
	RouteHostPort    string
	Basepath         string
	JaegerUI         string
}

// NewServer creates a new frontend.Server
func NewServer(options ConfigOptions, tracer trace.TracerProvider, logger log.Factory) *Server {
	return &Server{
		hostPort: options.FrontendHostPort,
		tracer:   tracer,
		logger:   logger,
		bestETA:  newBestETA(tracer, logger, options),
		assetFS:  httpfs.PrefixedFS("web_assets", http.FS(assetFS)),
		basepath: options.Basepath,
		jaegerUI: options.JaegerUI,
	}
}

// Run starts the frontend server
func (s *Server) Run() error {
	mux := s.createServeMux()
	s.logger.Bg().Info("Starting", zap.String("address", "http://"+path.Join(s.hostPort, s.basepath)))
	server := &http.Server{
		Addr:              s.hostPort,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	return server.ListenAndServe()
}

func (s *Server) createServeMux() http.Handler {
	mux := tracing.NewServeMux(true, s.tracer, s.logger)
	p := path.Join("/", s.basepath)
	mux.Handle(p, http.StripPrefix(p, http.FileServer(s.assetFS)))
	mux.Handle(path.Join(p, "/dispatch"), http.HandlerFunc(s.dispatch))
	mux.Handle(path.Join(p, "/config"), http.HandlerFunc(s.config))
	mux.Handle(path.Join(p, "/debug/vars"), expvar.Handler()) // expvar
	mux.Handle(path.Join(p, "/metrics"), promhttp.Handler())  // Prometheus
	return mux
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	config := map[string]string{
		"jaeger": s.jaegerUI,
	}
	s.writeResponse(config, w, r)
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.logger.For(ctx).Info("HTTP request received", zap.String("method", r.Method), zap.Stringer("url", r.URL))
	if err := r.ParseForm(); httperr.HandleError(w, err, http.StatusBadRequest) {
		s.logger.For(ctx).Error("bad request", zap.Error(err))
		return
	}

	customer := r.Form.Get("customer")
	if customer == "" {
		http.Error(w, "Missing required 'customer' parameter", http.StatusBadRequest)
		return
	}
	customerID, err := strconv.Atoi(customer)
	if err != nil {
		http.Error(w, "Parameter 'customer' is not an integer", http.StatusBadRequest)
		return
	}

	// TODO distinguish between user errors (such as invalid customer ID) and server failures
	response, err := s.bestETA.Get(ctx, customerID)
	if httperr.HandleError(w, err, http.StatusInternalServerError) {
		s.logger.For(ctx).Error("request failed", zap.Error(err))
		return
	}

	s.writeResponse(response, w, r)
}

func (s *Server) writeResponse(response any, w http.ResponseWriter, r *http.Request) {
	data, err := json.Marshal(response)
	if httperr.HandleError(w, err, http.StatusInternalServerError) {
		s.logger.For(r.Context()).Error("cannot marshal response", zap.Error(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}
