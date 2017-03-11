// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package app

import (
	"time"

	"github.com/uber-go/zap"
	"github.com/uber/jaeger/model/adjuster"
	"github.com/uber/jaeger/storage/spanstore"
)

// HandlerOption is a function that sets some option on the APIHandler
type HandlerOption func(handler *APIHandler)

// HandlerOptions is a factory for all available HandlerOptions
var HandlerOptions handlerOptions

type handlerOptions struct{}

// Logger creates a HandlerOption that initializes Logger on the APIHandler,
// which is used to emit logs.
func (handlerOptions) Logger(logger zap.Logger) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.logger = logger
	}
}

// Adjusters creates a HandlerOption that initializes the sequence of Adjusters on the APIHandler,
func (handlerOptions) Adjusters(adjusters []adjuster.Adjuster) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.adjuster = adjuster.Sequence(adjusters...)
	}
}

// Prefix creates a HandlerOption that initializes prefix HTTP prefix of the API
func (handlerOptions) Prefix(prefix string) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.httpPrefix = prefix
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
		apiHandler.archiveSpanReader = reader
	}
}

// ArchiveSpanWriter creates a HandlerOption that initializes lookback duration
func (handlerOptions) ArchiveSpanWriter(writer spanstore.Writer) HandlerOption {
	return func(apiHandler *APIHandler) {
		apiHandler.archiveSpanWriter = writer
	}
}
