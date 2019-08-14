// Copyright (c) 2018 The Jaeger Authors.
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

package grpc

import (
	"crypto/x509"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	grpcManager "github.com/jaegertracing/jaeger/cmd/agent/app/configmanager/grpc"
	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	aReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
)

// ProxyBuilder holds objects communicating with collector
type ProxyBuilder struct {
	reporter aReporter.Reporter
	manager  configmanager.ClientConfigManager
	conn     *grpc.ClientConn
}

var systemCertPool = x509.SystemCertPool // to allow overriding in unit test

// NewCollectorProxy creates ProxyBuilder
func NewCollectorProxy(builder *ConnBuilder, agentTags map[string]string, mFactory metrics.Factory, logger *zap.Logger) (*ProxyBuilder, error) {
	conn, err := builder.CreateConnection(logger)
	if err != nil {
		return nil, err
	}
	grpcMetrics := mFactory.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{"protocol": "grpc"}})
	return &ProxyBuilder{
		conn:     conn,
		reporter: reporter.WrapWithQueue(reporter.WrapWithMetrics(NewReporter(conn, agentTags, logger), grpcMetrics), logger),
		manager:  configmanager.WrapWithMetrics(grpcManager.NewConfigManager(conn), grpcMetrics),
	}, nil
}

// GetConn returns grpc conn
func (b ProxyBuilder) GetConn() *grpc.ClientConn {
	return b.conn
}

// GetReporter returns Reporter
func (b ProxyBuilder) GetReporter() aReporter.Reporter {
	return b.reporter
}

// GetManager returns manager
func (b ProxyBuilder) GetManager() configmanager.ClientConfigManager {
	return b.manager
}

// Close closes connections used by proxy.
func (b ProxyBuilder) Close() error {
	return b.conn.Close()
}
