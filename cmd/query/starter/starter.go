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

package starter

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	basic "github.com/uber/jaeger/cmd/builder"
	queryApp "github.com/uber/jaeger/cmd/query/app"
	query "github.com/uber/jaeger/cmd/query/app/builder"
	"github.com/uber/jaeger/pkg/recoveryhandler"
	"github.com/uber/jaeger/storage/spanstore/memory"
)

// StartQuery starts a new query service, based on the given parameters.
func StartQuery(logger *zap.Logger, baseFactory metrics.Factory, memoryStore *memory.Store) {
	metricsFactory := baseFactory.Namespace("jaeger-query", nil)

	storageBuild, err := query.NewStorageBuilder(
		basic.Options.LoggerOption(logger),
		basic.Options.MetricsFactoryOption(metricsFactory),
		basic.Options.MemoryStoreOption(memoryStore),
	)
	if err != nil {
		logger.Fatal("Failed to wire up service", zap.Error(err))
	}
	spanReader, err := storageBuild.NewSpanReader()
	if err != nil {
		logger.Fatal("Failed to get span reader", zap.Error(err))
	}
	dependencyReader, err := storageBuild.NewDependencyReader()
	if err != nil {
		logger.Fatal("Failed to get dependency reader", zap.Error(err))
	}
	rHandler := queryApp.NewAPIHandler(
		spanReader,
		dependencyReader,
		queryApp.HandlerOptions.Prefix(*query.QueryPrefix),
		queryApp.HandlerOptions.Logger(logger))
	sHandler := queryApp.NewStaticAssetsHandler(*query.QueryStaticAssets)
	r := mux.NewRouter()
	rHandler.RegisterRoutes(r)
	sHandler.RegisterRoutes(r)
	portStr := ":" + strconv.Itoa(*query.QueryPort)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)
	logger.Info("Starting jaeger-query HTTP server", zap.Int("port", *query.QueryPort))
	if err := http.ListenAndServe(portStr, recoveryHandler(r)); err != nil {
		logger.Fatal("Could not launch service", zap.Error(err))
	}
}
