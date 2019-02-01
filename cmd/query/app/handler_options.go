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
	"time"

	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/storage/spanstore"
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
		apiHandler.queryService.logger = logger
	}
}

// Adjusters creates a HandlerOption that initializes the sequence of Adjusters on the APIHandler,
func (handlerOptions) Adjusters(adjusters ...adjuster.Adjuster) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.queryService.adjuster = adjuster.Sequence(adjusters...)
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

// ArchiveSpanReader creates a HandlerOption that initializes lookback duration
func (handlerOptions) ArchiveSpanReader(reader spanstore.Reader) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.queryService.archiveSpanReader = reader
	}
}

// ArchiveSpanWriter creates a HandlerOption that initializes lookback duration
func (handlerOptions) ArchiveSpanWriter(writer spanstore.Writer) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.queryService.archiveSpanWriter = writer
	}
}

// Tracer creates a HandlerOption that initializes OpenTracing tracer
func (handlerOptions) Tracer(tracer opentracing.Tracer) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.queryService.tracer = tracer
	}
}
