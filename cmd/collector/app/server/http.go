// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/server/httpmetrics"
	"github.com/jaegertracing/jaeger/internal/healthcheck"
	"github.com/jaegertracing/jaeger/internal/recoveryhandler"
	samplinghttp "github.com/jaegertracing/jaeger/internal/sampling/http"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
)

// HTTPServerParams to construct a new Jaeger Collector HTTP Server
type HTTPServerParams struct {
	confighttp.ServerConfig
	Handler          handler.JaegerBatchesHandler
	SamplingProvider samplingstrategy.Provider
	MetricsFactory   metrics.Factory
	HealthCheck      *healthcheck.HealthCheck
	Logger           *zap.Logger
}

// StartHTTPServer based on the given parameters
func StartHTTPServer(params *HTTPServerParams) (*http.Server, error) {
	params.Logger.Info("Starting jaeger-collector HTTP server", zap.String("host-port", params.Endpoint))
	listener, err := params.ToListener(context.Background())
	if err != nil {
		return nil, err
	}
	settings := telemetry.NoopSettings().ToOtelComponent()
	settings.Logger = params.Logger
	server, err := params.ToServer(context.Background(), nil, settings, nil)
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

	cfgHandler := samplinghttp.NewHandler(samplinghttp.HandlerParams{
		ConfigManager: &samplinghttp.ConfigManager{
			SamplingProvider: params.SamplingProvider,
		},
		MetricsFactory:         params.MetricsFactory,
		BasePath:               "/api",
		LegacySamplingEndpoint: false,
	})
	cfgHandler.RegisterRoutes(r)

	recoveryHandler := recoveryhandler.NewRecoveryHandler(params.Logger, true)
	server.Handler = httpmetrics.Wrap(recoveryHandler(r), params.MetricsFactory, params.Logger)
	go func() {
		err := server.Serve(listener)
		if err != nil {
			if err != http.ErrServerClosed {
				params.Logger.Error("Could not start HTTP collector", zap.Error(err))
			}
		}
		params.HealthCheck.Set(healthcheck.Unavailable)
	}()
}
