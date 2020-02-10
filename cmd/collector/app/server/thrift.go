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
	"strconv"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling"
	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/strategystore"
	jc "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	sc "github.com/jaegertracing/jaeger/thrift-gen/sampling"
	zc "github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// ThriftServerParams to construct a new Jaeger Collector Thrift Server
type ThriftServerParams struct {
	JaegerBatchesHandler handler.JaegerBatchesHandler
	ZipkinSpansHandler   handler.ZipkinSpansHandler
	StrategyStore        strategystore.StrategyStore
	ServiceName          string
	Port                 int
	Logger               *zap.Logger
}

// StartThriftServer based on the given parameters
func StartThriftServer(params *ThriftServerParams) (*tchannel.Channel, error) {
	portStr := ":" + strconv.Itoa(params.Port)
	listener, err := net.Listen("tcp", portStr)
	if err != nil {
		params.Logger.Fatal("Unable to start listening on channel", zap.Error(err))
		return nil, err
	}

	var tchServer *tchannel.Channel
	if tchServer, err = tchannel.NewChannel(params.ServiceName, &tchannel.ChannelOptions{}); err != nil {
		params.Logger.Fatal("Unable to create new TChannel", zap.Error(err))
		return nil, err
	}

	if err := serveThrift(tchServer, listener, params); err != nil {
		return nil, err
	}

	return tchServer, nil
}

func serveThrift(tchServer *tchannel.Channel, listener net.Listener, params *ThriftServerParams) error {
	server := thrift.NewServer(tchServer)
	batchHandler := handler.NewTChannelHandler(params.JaegerBatchesHandler, params.ZipkinSpansHandler)
	server.Register(jc.NewTChanCollectorServer(batchHandler))
	server.Register(zc.NewTChanZipkinCollectorServer(batchHandler))
	server.Register(sc.NewTChanSamplingManagerServer(sampling.NewHandler(params.StrategyStore)))

	params.Logger.Info("Starting jaeger-collector TChannel server", zap.Int("port", params.Port))
	params.Logger.Warn("TChannel has been deprecated and will be removed in a future release")

	if err := tchServer.Serve(listener); err != nil {
		return err
	}

	return nil
}
