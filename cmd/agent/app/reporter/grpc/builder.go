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
	"errors"
	"strings"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/naming"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
)

// Builder Struct to hold configurations
type ConnBuilder struct {
	// CollectorHostPorts is list of host:port Jaeger Collectors.
	CollectorHostPorts []string `yaml:"collectorHostPorts"`

	// Resolver is custom resolver provided to grpc client balancer
	Resolver naming.Resolver

	// ResolverTarget is the external discovery service path for collectors
	ResolverTarget string `yaml:"resolverTarget"`

	MaxRetry      uint
	TLS           bool
	TLSCA         string
	TLSServerName string
}

// NewConnBuilder creates a new grpc connection builder.
func NewConnBuilder() *ConnBuilder {
	return &ConnBuilder{}
}

// CreateConnection creates the gRPC connection
func (b *ConnBuilder) CreateConnection(logger *zap.Logger) (*grpc.ClientConn, error) {
	var dialOptions []grpc.DialOption
	var dialTarget string
	if b.TLS { // user requested a secure connection
		logger.Info("Agent requested secure grpc connection to collector(s)")
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
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(creds))
	} else { // insecure connection
		logger.Info("Agent requested insecure grpc connection to collector(s)")
		dialOptions = append(dialOptions, grpc.WithInsecure())
	}

	if b.Resolver != nil && b.ResolverTarget != "" {
		// Use custom Resolver and ResolverTarget if specified
		logger.Info("Agent is using external resolver with roundrobin load balancer", zap.String("resolverTarget", b.ResolverTarget))
		dialTarget = b.ResolverTarget
		dialOptions = append(dialOptions, grpc.WithBalancer(grpc.RoundRobin(b.Resolver)))
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
			r.InitialAddrs(resolvedAddrs)
			dialTarget = r.Scheme() + ":///round_robin"
			logger.Info("Agent is connecting to a static list of collectors", zap.String("dialTarget", dialTarget), zap.String("collector hosts", strings.Join(b.CollectorHostPorts, ",")))
		} else {
			dialTarget = b.CollectorHostPorts[0]
		}
		dialOptions = append(dialOptions, grpc.WithBalancerName(roundrobin.Name))
	}

	dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(grpc_retry.WithMax(b.MaxRetry))))
	return grpc.Dial(dialTarget, dialOptions...)
}
