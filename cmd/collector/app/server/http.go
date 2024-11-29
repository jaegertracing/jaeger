// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/samplingstrategy"
	clientcfgHandler "github.com/jaegertracing/jaeger/pkg/clientcfg/clientcfghttp"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/httpmetrics"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
)

// HTTPServerParams to construct a new Jaeger Collector HTTP Server
type HTTPServerParams struct {
	TLSConfig        *configtls.ServerConfig
	HostPort         string
	Handler          handler.JaegerBatchesHandler
	SamplingProvider samplingstrategy.Provider
	MetricsFactory   metrics.Factory
	HealthCheck      *healthcheck.HealthCheck
	Logger           *zap.Logger

	// ReadTimeout sets the respective parameter of http.Server
	ReadTimeout time.Duration
	// ReadHeaderTimeout sets the respective parameter of http.Server
	ReadHeaderTimeout time.Duration
	// IdleTimeout sets the respective parameter of http.Server
	IdleTimeout time.Duration
}

type httpServer struct {
	*http.Server
	staticHandlerCloser io.Closer
}

// StartHTTPServer based on the given parameters
func StartHTTPServer(params *HTTPServerParams) (*http.Server, error) {
	params.Logger.Info("Starting jaeger-collector HTTP server", zap.String("http host-port", params.HostPort))

	errorLog, _ := zap.NewStdLogAt(params.Logger, zapcore.ErrorLevel)
	server := &http.Server{
		Addr:              params.HostPort,
		ReadTimeout:       params.ReadTimeout,
		ReadHeaderTimeout: params.ReadHeaderTimeout,
		IdleTimeout:       params.IdleTimeout,
		ErrorLog:          errorLog,
	}
	if params.TLSConfig != nil {
		tlsCfg, err := params.TLSConfig.LoadTLSConfig(context.Background())
		if err != nil {
			return nil, err
		}
		server.TLSConfig = tlsCfg
	}

	listener, err := net.Listen("tcp", params.HostPort)
	if err != nil {
		return nil, err
	}

	serveHTTP(server, listener, params)

	return server, nil
}

func serveHTTP(server *http.Server, listener net.Listener, params *HTTPServerParams) {
	r := mux.NewRouter()
	apiHandler := handler.NewAPIHandler(params.Handler)
	apiHandler.RegisterRoutes(r)

	cfgHandler := clientcfgHandler.NewHTTPHandler(clientcfgHandler.HTTPHandlerParams{
		ConfigManager: &clientcfgHandler.ConfigManager{
			SamplingProvider: params.SamplingProvider,
			// TODO provide baggage manager
		},
		MetricsFactory:         params.MetricsFactory,
		BasePath:               "/api",
		LegacySamplingEndpoint: false,
	})
	cfgHandler.RegisterRoutes(r)

	recoveryHandler := recoveryhandler.NewRecoveryHandler(params.Logger, true)
	server.Handler = httpmetrics.Wrap(recoveryHandler(r), params.MetricsFactory, params.Logger)
	go func() {
		var err error
		if params.TLSConfig != nil {
			err = server.ServeTLS(listener, "", "")
		} else {
			err = server.Serve(listener)
		}
		if err != nil {
			if err != http.ErrServerClosed {
				params.Logger.Error("Could not start HTTP collector", zap.Error(err))
			}
		}
		params.HealthCheck.Set(healthcheck.Unavailable)
	}()
}
