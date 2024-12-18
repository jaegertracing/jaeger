// Copyright (c) 2018-2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/retry"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter"
	"github.com/jaegertracing/jaeger/pkg/discovery"
	"github.com/jaegertracing/jaeger/pkg/discovery/grpcresolver"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/netutils"
)

// ConnBuilder Struct to hold configurations
type ConnBuilder struct {
	// CollectorHostPorts is list of host:port Jaeger Collectors.
	CollectorHostPorts []string `yaml:"collectorHostPorts"`
	TLS                *configtls.ClientConfig
	MaxRetry           uint
	DiscoveryMinPeers  int
	Notifier           discovery.Notifier
	Discoverer         discovery.Discoverer

	AdditionalDialOptions []grpc.DialOption
}

// NewConnBuilder creates a new grpc connection builder.
func NewConnBuilder() *ConnBuilder {
	return &ConnBuilder{}
}

// CreateConnection creates the gRPC connection
func (b *ConnBuilder) CreateConnection(ctx context.Context, logger *zap.Logger, mFactory metrics.Factory) (*grpc.ClientConn, error) {
	var dialOptions []grpc.DialOption
	var dialTarget string
	if b.TLS != nil { // user requested a secure connection
		logger.Info("Agent requested secure grpc connection to collector(s)")
		tlsConf, err := b.TLS.LoadTLSConfig(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load TLS config: %w", err)
		}
		creds := credentials.NewTLS(tlsConf)
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	} else { // insecure connection
		logger.Info("Agent requested insecure grpc connection to collector(s)")
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if b.Notifier != nil && b.Discoverer != nil {
		logger.Info("Using external discovery service with roundrobin load balancer")
		grpcResolver := grpcresolver.New(b.Notifier, b.Discoverer, logger, b.DiscoveryMinPeers)
		dialTarget = grpcResolver.Scheme() + ":///round_robin"
	} else {
		if b.CollectorHostPorts == nil {
			return nil, errors.New("at least one collector hostPort address is required when resolver is not available")
		}
		b.CollectorHostPorts = netutils.FixLocalhost(b.CollectorHostPorts)
		if len(b.CollectorHostPorts) > 1 {
			r := manual.NewBuilderWithScheme("jaeger-manual")
			dialOptions = append(dialOptions, grpc.WithResolvers(r))
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
	dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(retry.UnaryClientInterceptor(retry.WithMax(b.MaxRetry))))
	dialOptions = append(dialOptions, b.AdditionalDialOptions...)

	conn, err := grpc.NewClient(dialTarget, dialOptions...)
	if err != nil {
		return nil, err
	}

	connectMetrics := reporter.NewConnectMetrics(
		mFactory.Namespace(metrics.NSOptions{Tags: map[string]string{"protocol": "grpc"}}),
	)

	go func(cc *grpc.ClientConn, cm *reporter.ConnectMetrics) {
		logger.Info("Checking connection to collector")

		for {
			select {
			case <-ctx.Done():
				logger.Info("Stopping connection")
				return
			default:
				s := cc.GetState()
				if s == connectivity.Ready {
					cm.OnConnectionStatusChange(true)
					cm.RecordTarget(cc.Target())
				} else {
					cm.OnConnectionStatusChange(false)
				}

				logger.Info("Agent collector connection state change", zap.String("dialTarget", dialTarget), zap.Stringer("status", s))
				cc.WaitForStateChange(ctx, s)
			}
		}
	}(conn, connectMetrics)

	return conn, nil
}
