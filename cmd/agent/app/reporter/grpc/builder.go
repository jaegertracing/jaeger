// Copyright (c) 2018-2019 The Jaeger Authors.
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
	"context"
	"errors"
	"expvar"
	"fmt"
	"github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"strings"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/discovery"
	"github.com/jaegertracing/jaeger/pkg/discovery/grpcresolver"
)

// ConnBuilder Struct to hold configurations
type ConnBuilder struct {
	// CollectorHostPorts is list of host:port Jaeger Collectors.
	CollectorHostPorts []string `yaml:"collectorHostPorts"`

	MaxRetry uint
	TLS      tlscfg.Options

	DiscoveryMinPeers int
	Notifier          discovery.Notifier
	Discoverer        discovery.Discoverer

	ConnectMetricsParams *reporter.ConnectMetricsParams
}

// NewConnBuilder creates a new grpc connection builder.
func NewConnBuilder() *ConnBuilder {
	return &ConnBuilder{}
}

// CreateConnection creates the gRPC connection
func (b *ConnBuilder) CreateConnection(logger *zap.Logger, mFactory metrics.Factory) (*grpc.ClientConn, error) {
	var dialOptions []grpc.DialOption
	var dialTarget string
	if b.TLS.Enabled { // user requested a secure connection
		logger.Info("Agent requested secure grpc connection to collector(s)")
		tlsConf, err := b.TLS.Config(logger)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}

		creds := credentials.NewTLS(tlsConf)
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	} else { // insecure connection
		logger.Info("Agent requested insecure grpc connection to collector(s)")
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}

	if b.Notifier != nil && b.Discoverer != nil {
		logger.Info("Using external discovery service with roundrobin load balancer")
		grpcResolver := grpcresolver.New(b.Notifier, b.Discoverer, logger, b.DiscoveryMinPeers)
		dialTarget = grpcResolver.Scheme() + ":///round_robin"
	} else {
		if b.CollectorHostPorts == nil {
			return nil, errors.New("at least one collector hostPort address is required when resolver is not available")
		}
		if len(b.CollectorHostPorts) > 1 {
			r, _ := manual.GenerateAndRegisterManualResolver()
			var resolvedAddrs []resolver.Address
			for _, addr := range b.CollectorHostPorts {
				resolvedAddrs = append(resolvedAddrs, resolver.Address{Addr: addr})
			}
			r.InitialState(resolver.State{Addresses: resolvedAddrs})
			dialTarget = r.Scheme() + ":///round_robin"
			logger.Info("Agent is connecting to a static list of collectors", zap.String("dialTarget", dialTarget), zap.String("collector hosts", strings.Join(b.CollectorHostPorts, ",")))
		} else {
			dialTarget = b.CollectorHostPorts[0]
		}
	}
	dialOptions = append(dialOptions, grpc.WithDefaultServiceConfig(grpcresolver.GRPCServiceConfig))
	dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(grpc_retry.WithMax(b.MaxRetry))))
	conn, err := grpc.Dial(dialTarget, dialOptions...)

	if err != nil {
		return nil, err
	}

	if b.ConnectMetricsParams == nil {
		grpcMetrics := mFactory.Namespace(metrics.NSOptions{Name: "", Tags: map[string]string{"protocol": "grpc"}})

		// for unit test and provide ConnectMetricsParams and outside call
		cmp := reporter.NewConnectMetrics(reporter.ConnectMetricsParams{
			Logger:         logger,
			MetricsFactory: grpcMetrics,
		})
		b.ConnectMetricsParams = cmp
	}

	go func(cc *grpc.ClientConn, cm *reporter.ConnectMetricsParams) {
		logger.Info("Checking connection to collector")

		var egt *expvar.String
		r := expvar.Get("gRPCTarget")
		if r == nil{
			egt = expvar.NewString("gRPCTarget")
		}else {
			egt = r.(*expvar.String)
		}

		for {
			s := cc.GetState()
			if s == connectivity.Ready {
				cm.OnConnectionStatusChange(true)
				egt.Set(cc.Target())
			} else {
				cm.OnConnectionStatusChange(false)
			}

			logger.Info("Agent collector connection state change", zap.String("dialTarget", dialTarget), zap.Stringer("status", s))
			cc.WaitForStateChange(context.Background(), s)
		}
	}(conn, b.ConnectMetricsParams)

	return conn, nil
}
