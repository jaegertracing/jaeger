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
	"net"
	"strconv"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	basic "github.com/uber/jaeger/cmd/builder"
	collector "github.com/uber/jaeger/cmd/collector/app/builder"
	"github.com/uber/jaeger/storage/spanstore/memory"
	jc "github.com/uber/jaeger/thrift-gen/jaeger"
	zc "github.com/uber/jaeger/thrift-gen/zipkincore"
)

const (
	serviceName = "jaeger-collector"
)

// StartCollector starts a new collector service, based on the given parameters.
func StartCollector(logger *zap.Logger, baseFactory metrics.Factory, memoryStore *memory.Store) {
	metricsFactory := baseFactory.Namespace(serviceName, nil)

	spanBuilder, err := collector.NewSpanHandlerBuilder(
		basic.Options.LoggerOption(logger),
		basic.Options.MetricsFactoryOption(metricsFactory),
		basic.Options.MemoryStoreOption(memoryStore),
	)
	if err != nil {
		logger.Fatal("Unable to set up builder", zap.Error(err))
	}
	zipkinSpansHandler, jaegerBatchesHandler, err := spanBuilder.BuildHandlers()
	if err != nil {
		logger.Fatal("Unable to build span handlers", zap.Error(err))
	}

	ch, err := tchannel.NewChannel("jaeger-collector", &tchannel.ChannelOptions{})
	if err != nil {
		logger.Fatal("Unable to create new TChannel", zap.Error(err))
	}
	server := thrift.NewServer(ch)
	server.Register(jc.NewTChanCollectorServer(jaegerBatchesHandler))
	server.Register(zc.NewTChanZipkinCollectorServer(zipkinSpansHandler))
	portStr := ":" + strconv.Itoa(*collector.CollectorPort)
	listener, err := net.Listen("tcp", portStr)
	if err != nil {
		logger.Fatal("Unabled to start listening on channel", zap.Error(err))
	}
	ch.Serve(listener)
	logger.Info("Starting jaeger-collector TChannel server", zap.Int("port", *collector.CollectorPort))
}
