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
	"context"
	"errors"
	"fmt"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
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

// NewCollectorProxy creates ProxyBuilder
func NewCollectorProxy(o *Options, mFactory metrics.Factory, logger *zap.Logger) (*ProxyBuilder, error) {
	if len(o.CollectorHostPort) == 0 {
		return nil, errors.New("could not create collector proxy, address is missing")
	}

	opts := []grpc.DialOption{
		//https://github.com/grpc/grpc-go/issues/2443
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			// After a duration of this time if the client doesn't see any activity it pings the server to see if the transport is still alive.
			Time: 2 * time.Minute, // The current default value is infinity.
			// After having pinged for keepalive check, the client waits for a duration of Timeout and if no activity is seen even after that
			// the connection is closed.
			Timeout: 20 * time.Second, // The current default value is 20s.
			// If true, client runs keepalive checks even with no active RPCs.
			PermitWithoutStream: true,
		}),
	}
	if o.TLSServerCertPath != "" {
		creds, err := credentials.NewClientTLSFromFile(o.TLSServerCertPath, "")
		if err != nil {
			fmt.Printf("cert load error: %s\n", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	var conn *grpc.ClientConn
	var err error
	if len(o.CollectorHostPort) > 1 {
		r, _ := manual.GenerateAndRegisterManualResolver()
		var resolvedAddrs []resolver.Address
		for _, addr := range o.CollectorHostPort {
			resolvedAddrs = append(resolvedAddrs, resolver.Address{Addr: addr})
		}
		r.InitialAddrs(resolvedAddrs)
		opts = append(opts, grpc.WithBalancerName(roundrobin.Name))
		conn, _ = grpc.Dial(r.Scheme()+":///round_robin", opts...)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if conn, err = grpc.DialContext(ctx, o.CollectorHostPort[0], opts...); err != nil {
			return nil, fmt.Errorf("error dialing collector: %v", err)
		}
	}
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
