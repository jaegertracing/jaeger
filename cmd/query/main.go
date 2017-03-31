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

package main

import (
	"flag"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/uber-go/zap"

	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	"github.com/uber/jaeger/cmd/query/app"

	basicB "github.com/uber/jaeger/cmd/builder"
	casFlags "github.com/uber/jaeger/cmd/flags/cassandra"
	"github.com/uber/jaeger/cmd/query/app/builder"
	"github.com/uber/jaeger/pkg/recoveryhandler"
)

func main() {
	casOptions := casFlags.NewOptions()
	casOptions.Bind(flag.CommandLine, "cassandra", "cassandra.archive")
	flag.Parse()

	logger := zap.New(zap.NewJSONEncoder())
	metricsFactory := xkit.Wrap("jaeger-query", expvar.NewFactory(10))

	storageBuild, err := builder.NewStorageBuilder(
		basicB.Options.LoggerOption(logger),
		basicB.Options.MetricsFactoryOption(metricsFactory),
		basicB.Options.CassandraOption(casOptions.GetPrimary()),
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
	rHandler := app.NewAPIHandler(
		spanReader,
		dependencyReader,
		app.HandlerOptions.Adjusters(app.StandardAdjusters),
		app.HandlerOptions.Prefix(*builder.QueryPrefix),
		app.HandlerOptions.Logger(logger))
	r := mux.NewRouter()
	rHandler.RegisterRoutes(r)
	portStr := ":" + strconv.Itoa(*builder.QueryPort)
	recoveryHandler := recoveryhandler.NewRecoveryHandler(logger, true)
	logger.Info("Starting jaeger-query HTTP server", zap.Int("port", *builder.QueryPort))
	if err := http.ListenAndServe(portStr, recoveryHandler(r)); err != nil {
		logger.Fatal("Could not launch service", zap.Error(err))
	}
}
