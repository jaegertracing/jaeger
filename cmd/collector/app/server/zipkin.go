// Copyright (c) 2020 The Jaeger Authors.
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

package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/httpmetrics"
	"github.com/jaegertracing/jaeger/pkg/recoveryhandler"
)

// ZipkinServerParams to construct a new Jaeger Collector Zipkin Server
type ZipkinServerParams struct {
	HostPort       string
	Handler        handler.ZipkinSpansHandler
	AllowedOrigins string
	AllowedHeaders string
	HealthCheck    *healthcheck.HealthCheck
	Logger         *zap.Logger
	MetricsFactory metrics.Factory
}

// StartZipkinServer based on the given parameters
func StartZipkinServer(params *ZipkinServerParams) (*http.Server, error) {
	if params.HostPort == "" {
		params.Logger.Info("Not listening for Zipkin HTTP traffic, port not configured")
		return nil, nil
	}

	params.Logger.Info("Listening for Zipkin HTTP traffic", zap.String("zipkin host-port", params.HostPort))

	listener, err := net.Listen("tcp", params.HostPort)
	if err != nil {
		return nil, err
	}

	server := &http.Server{Addr: params.HostPort}
	serveZipkin(server, listener, params)

	return server, nil
}

func serveZipkin(server *http.Server, listener net.Listener, params *ZipkinServerParams) {
	r := mux.NewRouter()
	zHandler := zipkin.NewAPIHandler(params.Handler)
	zHandler.RegisterRoutes(r)

	origins := strings.Split(strings.ReplaceAll(params.AllowedOrigins, " ", ""), ",")
	headers := strings.Split(strings.ReplaceAll(params.AllowedHeaders, " ", ""), ",")

	cors := cors.New(cors.Options{
		AllowedOrigins: origins,
		AllowedMethods: []string{"POST"}, // Allowing only POST, because that's the only handled one
		AllowedHeaders: headers,
	})

	recoveryHandler := recoveryhandler.NewRecoveryHandler(params.Logger, true)
	server.Handler = cors.Handler(httpmetrics.Wrap(recoveryHandler(r), params.MetricsFactory))
	go func(listener net.Listener, server *http.Server) {
		if err := server.Serve(listener); err != nil {
			if err != http.ErrServerClosed {
				params.Logger.Error("Could not launch Zipkin server", zap.Error(err))
			}
		}
		params.HealthCheck.Set(healthcheck.Unavailable)
	}(listener, server)
}
