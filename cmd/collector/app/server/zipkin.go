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
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
)

// ZipkinServerParams to construct a new Jaeger Collector Zipkin Server
type ZipkinServerParams struct {
	Port            int
	Handler         handler.ZipkinSpansHandler
	RecoveryHandler func(http.Handler) http.Handler
	AllowedOrigins  string
	AllowedHeaders  string
	HealthCheck     *healthcheck.HealthCheck
	Logger          *zap.Logger
}

// StartZipkinServer based on the given parameters
func StartZipkinServer(params *ZipkinServerParams) (*http.Server, error) {
	if params.Port == 0 {
		return nil, nil
	}

	httpPortStr := ":" + strconv.Itoa(params.Port)
	params.Logger.Info("Listening for Zipkin HTTP traffic", zap.Int("zipkin.http-port", params.Port))

	listener, err := net.Listen("tcp", httpPortStr)
	if err != nil {
		return nil, err
	}

	server := &http.Server{Addr: httpPortStr}
	if err := serveZipkin(server, listener, params); err != nil {
		return nil, err
	}

	return server, nil
}

func serveZipkin(server *http.Server, listener net.Listener, params *ZipkinServerParams) error {
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

	server.Handler = cors.Handler(params.RecoveryHandler(r))
	go func(listener net.Listener, server *http.Server) {
		if err := server.Serve(listener); err != nil {
			params.Logger.Fatal("Could not launch Zipkin server", zap.Error(err))
		}
		params.HealthCheck.Set(healthcheck.Unavailable)
	}(listener, server)

	return nil
}
