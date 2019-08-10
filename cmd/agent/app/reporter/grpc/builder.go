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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/jaegertracing/jaeger/pkg/discovery"
	"github.com/jaegertracing/jaeger/pkg/discovery/grpcresolver"
)

// ConnBuilder Struct to hold configurations
type ConnBuilder struct {
	// CollectorHostPorts is list of host:port Jaeger Collectors.
	CollectorHostPorts []string `yaml:"collectorHostPorts"`

	MaxRetry      uint
	TLS           bool
	TLSCA         string
	TLSServerName string
	TLSCert       string
	TLSKey        string

	DiscoveryMinPeers int
	Notifier          discovery.Notifier
	Discoverer        discovery.Discoverer
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
		var err error
		var certPool *x509.CertPool
		if len(b.TLSCA) == 0 { // no truststore given, use SystemCertPool
			certPool, err = systemCertPool()
			if err != nil {
				return nil, err
			}
		} else { // setup user specified truststore
			caPEM, err := ioutil.ReadFile(b.TLSCA)
			if err != nil {
				return nil, fmt.Errorf("reading client CA failed, %v", err)
			}

			certPool = x509.NewCertPool()
			if !certPool.AppendCertsFromPEM(caPEM) {
				return nil, fmt.Errorf("building client CA failed, %v", err)
			}
		}

		tlsCfg := &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    certPool,
			ServerName: b.TLSServerName,
		}

		if (b.TLSKey == "" || b.TLSCert == "") &&
			(b.TLSKey != "" || b.TLSCert != "") {
			return nil, fmt.Errorf("for client auth, both client certificate and key must be supplied")
		}

		if b.TLSKey != "" && b.TLSCert != "" {
			tlsCert, err := tls.LoadX509KeyPair(b.TLSCert, b.TLSKey)
			if err != nil {
				return nil, fmt.Errorf("could not load server TLS cert and key, %v", err)
			}

			logger.Info("client TLS authentication enabled")
			tlsCfg.Certificates = []tls.Certificate{tlsCert}
		}

		creds := credentials.NewTLS(tlsCfg)
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
	dialOptions = append(dialOptions, grpc.WithBalancerName(roundrobin.Name))
	dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor(grpc_retry.WithMax(b.MaxRetry))))
	return grpc.Dial(dialTarget, dialOptions...)
}
