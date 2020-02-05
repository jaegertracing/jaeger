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

package app

import (
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"
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

// Tracer creates a HandlerOption that initializes OpenTracing tracer
func (handlerOptions) Tracer(tracer opentracing.Tracer) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.tracer = tracer
	}
}

// AdditionalHeaders creates a HandlerOption that adds abitrary Response Headers
func (handlerOptions) AdditionalHeaders(additionalHeaders http.Header) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.additionalHeaders = additionalHeaders
	}
}
