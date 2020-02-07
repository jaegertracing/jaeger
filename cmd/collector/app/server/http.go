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
	"strconv"

	"github.com/gorilla/mux"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	clientcfgHandler "github.com/jaegertracing/jaeger/pkg/clientcfg/clientcfghttp"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
)

// HTTPServerParams to construct a new Jaeger Collector HTTP Server
type HTTPServerParams struct {
	Port            int
	Handler         handler.JaegerBatchesHandler
	RecoveryHandler func(http.Handler) http.Handler
	SamplingStore   strategystore.StrategyStore
	MetricsFactory  metrics.Factory
	HealthCheck     *healthcheck.HealthCheck
	Logger          *zap.Logger
}

// StartHTTPServer based on the given parameters
func StartHTTPServer(params *HTTPServerParams) (*http.Server, error) {
	r := mux.NewRouter()
	apiHandler := handler.NewAPIHandler(params.Handler)
	apiHandler.RegisterRoutes(r)

	cfgHandler := clientcfgHandler.NewHTTPHandler(clientcfgHandler.HTTPHandlerParams{
		ConfigManager: &clientcfgHandler.ConfigManager{
			SamplingStrategyStore: params.SamplingStore,
			// TODO provide baggage manager
		},
		MetricsFactory:         params.MetricsFactory,
		BasePath:               "/api",
		LegacySamplingEndpoint: false,
	})
	cfgHandler.RegisterRoutes(r)

	httpPortStr := ":" + strconv.Itoa(params.Port)
	params.Logger.Info("Starting jaeger-collector HTTP server", zap.String("http-host-port", httpPortStr))

	listener, err := net.Listen("tcp", httpPortStr)
	if err != nil {
		return nil, err
	}

	hServer := &http.Server{Addr: httpPortStr, Handler: params.RecoveryHandler(r)}
	go func(listener net.Listener, hServer *http.Server) {
		if err := hServer.Serve(listener); err != nil {
			if err != http.ErrServerClosed {
				params.Logger.Fatal("Could not start HTTP collector", zap.Error(err))
			}
		}
		params.HealthCheck.Set(healthcheck.Unavailable)
	}(listener, hServer)

	return hServer, nil
}
