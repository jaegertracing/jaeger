// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package httpserver

import (
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	"github.com/jaegertracing/jaeger/pkg/clientcfg/clientcfghttp"
	"github.com/jaegertracing/jaeger/pkg/metrics"
)

// NewHTTPServer creates a new server that hosts an HTTP/JSON endpoint for clients
// to query for sampling strategies and baggage restrictions.
func NewHTTPServer(hostPort string, manager configmanager.ClientConfigManager, mFactory metrics.Factory, logger *zap.Logger) *http.Server {
	handler := clientcfghttp.NewHTTPHandler(clientcfghttp.HTTPHandlerParams{
		ConfigManager:          manager,
		MetricsFactory:         mFactory,
		LegacySamplingEndpoint: true,
	})
	r := mux.NewRouter()
	handler.RegisterRoutes(r)
	errorLog, _ := zap.NewStdLogAt(logger, zapcore.ErrorLevel)
	return &http.Server{
		Addr:              hostPort,
		Handler:           r,
		ErrorLog:          errorLog,
		ReadHeaderTimeout: 2 * time.Second,
	}
}
