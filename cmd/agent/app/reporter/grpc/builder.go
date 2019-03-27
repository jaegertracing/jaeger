// Copyright (c) 2019 The Jaeger Authors.
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
	"errors"
	"time"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/naming"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
)

// Builder Struct to hold configurations
type Builder struct {
	// CollectorHostPorts are host:ports of a static list of Jaeger Collectors.
	CollectorHostPorts []string `yaml:"collectorHostPorts"`

	// CollectorServiceName is the name that Jaeger Collector's grpc server
	// responds to.
	CollectorServiceName string `yaml:"collectorServiceName"`

	// ConnCheckTimeout is the timeout used when establishing new connections.
	ConnCheckTimeout time.Duration

	// ReportTimeout is the timeout used when reporting span batches.
	ReportTimeout time.Duration

	// Resolver is custom resolver provided to grpc client balancer
	Resolver naming.Resolver

	// ResolverTarget is the dns path for collectors
	ResolverTarget string

	MaxRetry      uint
	TLS           bool
	TLSCA         string
	TLSServerName string
}

// NewBuilder creates a new reporter builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// CreateReporter creates the TChannel-based Reporter
func (b *Builder) CreateReporter(logger *zap.Logger, mFactory metrics.Factory, agentTags map[string]string) (*Reporter, error) {

	var dialOption grpc.DialOption
	if b.TLS { // user requested a secure connection
		var creds credentials.TransportCredentials
		if len(b.TLSCA) == 0 { // no truststore given, use SystemCertPool
			pool, err := systemCertPool()
			if err != nil {
				return nil, err
			}
			creds = credentials.NewClientTLSFromCert(pool, b.TLSServerName)
		} else { // setup user specified truststore
			var err error
			creds, err = credentials.NewClientTLSFromFile(b.TLSCA, b.TLSServerName)
			if err != nil {
				return nil, err
			}
		}
		dialOption = grpc.WithTransportCredentials(creds)
	} else { // insecure connection
		dialOption = grpc.WithInsecure()
	}

	if b.Resolver != nil && b.ResolverTarget != "" {
		// If both resolver&resolverTarget are specified, we will rely on it to resolve collector addresses
		conn, _ := grpc.Dial(b.ResolverTarget,
			dialOption,
			grpc.WithBalancer(grpc.RoundRobin(b.Resolver)),
			grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(grpc_retry.WithMax(b.MaxRetry))))

		return NewReporter(conn, agentTags, logger), nil
	}
	if b.CollectorHostPorts == nil {
		return nil, errors.New("at least one collector hostPort address is required when resolver is not available")
	}
	var target string
	if len(b.CollectorHostPorts) > 1 {
		r, _ := manual.GenerateAndRegisterManualResolver()
		var resolvedAddrs []resolver.Address
		for _, addr := range b.CollectorHostPorts {
			resolvedAddrs = append(resolvedAddrs, resolver.Address{Addr: addr})
		}
		r.InitialAddrs(resolvedAddrs)
		target = r.Scheme() + ":///round_robin"
	} else {
		target = b.CollectorHostPorts[0]
	}

	// It does not return error if the collector is not running
	conn, _ := grpc.Dial(target,
		dialOption,
		grpc.WithBalancerName(roundrobin.Name),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(grpc_retry.WithMax(b.MaxRetry))))
	return NewReporter(conn, agentTags, logger), nil
}
