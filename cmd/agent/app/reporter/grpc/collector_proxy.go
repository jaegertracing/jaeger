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
	"errors"

	"github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/jaegertracing/jaeger/cmd/agent/app/configmanager"
	grpcManager "github.com/jaegertracing/jaeger/cmd/agent/app/configmanager/grpc"
	aReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
)

// ProxyBuilder holds objects communicating with collector
type ProxyBuilder struct {
	reporter aReporter.Reporter
	manager  configmanager.ClientConfigManager
	conn     *grpc.ClientConn
}

var systemCertPool = x509.SystemCertPool

// NewCollectorProxy creates ProxyBuilder
func NewCollectorProxy(o *Options, mFactory metrics.Factory, logger *zap.Logger) (*ProxyBuilder, error) {
	if len(o.CollectorHostPort) == 0 {
		return nil, errors.New("could not create collector proxy, address is missing")
	}
	var dialOption grpc.DialOption
	if o.TLS { // user requested a secure connection
		var creds credentials.TransportCredentials
		if len(o.TLSCA) == 0 { // no truststore given, use SystemCertPool
			pool, err := systemCertPool()
			if err != nil {
				return nil, err
			}
			creds = credentials.NewClientTLSFromCert(pool, o.TLSServerName)
		} else { // setup user specified truststore
			var err error
			creds, err = credentials.NewClientTLSFromFile(o.TLSCA, o.TLSServerName)
			if err != nil {
				return nil, err
			}
		}
		dialOption = grpc.WithTransportCredentials(creds)
	} else { // insecure connection
		dialOption = grpc.WithInsecure()
	}

	var target string
	if len(o.CollectorHostPort) > 1 {
		r, _ := manual.GenerateAndRegisterManualResolver()
		var resolvedAddrs []resolver.Address
		for _, addr := range o.CollectorHostPort {
			resolvedAddrs = append(resolvedAddrs, resolver.Address{Addr: addr})
		}
		r.InitialAddrs(resolvedAddrs)
		target = r.Scheme() + ":///round_robin"
	} else {
		target = o.CollectorHostPort[0]
	}
	// It does not return error if the collector is not running
	conn, _ := grpc.Dial(target,
		dialOption,
		grpc.WithBalancerName(roundrobin.Name),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(grpc_retry.WithMax(o.MaxRetry))))
	grpcMetrics := mFactory.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{"protocol": "grpc"}})
	return &ProxyBuilder{
		conn:     conn,
		reporter: aReporter.WrapWithMetrics(NewReporter(conn, logger), grpcMetrics),
		manager:  configmanager.WrapWithMetrics(grpcManager.NewConfigManager(conn), grpcMetrics)}, nil
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
