// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/query/app/querysvc"
)

// HandlerOption is a function that sets some option on the APIHandler
type HandlerOption func(handler *APIHandler)

// HandlerOptions is a factory for all available HandlerOptions
var HandlerOptions handlerOptions

type handlerOptions struct{}

// Logger creates a HandlerOption that initializes Logger on the APIHandler,
// which is used to emit logs.
func (handlerOptions) Logger(logger *zap.Logger) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.logger = logger
	}
}

// BasePath creates a HandlerOption that initializes the base path for all HTTP routes
func (handlerOptions) BasePath(prefix string) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.basePath = prefix
	}
}

// Prefix creates a HandlerOption that initializes the HTTP prefix for API routes
func (handlerOptions) Prefix(prefix string) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.apiPrefix = prefix
	}
}

// QueryLookbackDuration creates a HandlerOption that initializes lookback duration
func (handlerOptions) QueryLookbackDuration(queryLookbackDuration time.Duration) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.queryParser.traceQueryLookbackDuration = queryLookbackDuration
	}
}

// Tracer creates a HandlerOption that passes the tracer to the handler
func (handlerOptions) Tracer(tracer trace.TracerProvider) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.tracer = tracer
	}
}

// MetricsQueryService creates a HandlerOption that initializes MetricsQueryService.
func (handlerOptions) MetricsQueryService(mqs querysvc.MetricsQueryService) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.metricsQueryService = mqs
	}
}
