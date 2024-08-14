// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file ex	cept in compliance with the License.
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
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
)

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	RemoteServerAddr     string `yaml:"server" mapstructure:"server"`
	RemoteTLS            tlscfg.Options
	RemoteConnectTimeout time.Duration `yaml:"connection-timeout" mapstructure:"connection-timeout"`
	TenancyOpts          tenancy.Options
}

type ConfigV2 struct {
	Tenancy                        tenancy.Options `mapstructure:"multi_tenancy"`
	configgrpc.ClientConfig        `mapstructure:",squash"`
	exporterhelper.TimeoutSettings `mapstructure:",squash"`
}

func DefaultConfigV2() ConfigV2 {
	return ConfigV2{
		TimeoutSettings: exporterhelper.TimeoutSettings{
			Timeout: defaultConnectionTimeout,
		},
	}
}

func (c *Configuration) TranslateToConfigV2() *ConfigV2 {
	return &ConfigV2{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint:   c.RemoteServerAddr,
			TLSSetting: c.RemoteTLS.ToOtelClientConfig(),
		},
		TimeoutSettings: exporterhelper.TimeoutSettings{
			Timeout: c.RemoteConnectTimeout,
		},
	}
}

// ClientPluginServices defines services plugin can expose and its capabilities
type ClientPluginServices struct {
	shared.PluginServices
	Capabilities shared.PluginCapabilities
	remoteConn   *grpc.ClientConn
}

// TODO move this to factory.go
func (c *ConfigV2) Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error) {
	telset := component.TelemetrySettings{
		Logger:         logger,
		TracerProvider: tracerProvider,
	}
	newClientFn := func(opts ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		return c.ToClientConn(context.Background(), componenttest.NewNopHost(), telset, opts...)
	}
	return newRemoteStorage(c, telset, newClientFn)
}

type newClientFn func(opts ...grpc.DialOption) (*grpc.ClientConn, error)

func newRemoteStorage(c *ConfigV2, telset component.TelemetrySettings, newClient newClientFn) (*ClientPluginServices, error) {
	opts := []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(telset.TracerProvider))),
	}
	if c.Auth != nil {
		return nil, fmt.Errorf("authenticator is not supported")
	}

	tenancyMgr := tenancy.NewManager(&c.Tenancy)
	if tenancyMgr.Enabled {
		opts = append(opts, grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tenancyMgr)))
		opts = append(opts, grpc.WithStreamInterceptor(tenancy.NewClientStreamInterceptor(tenancyMgr)))
	}

	remoteConn, err := newClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating remote storage client: %w", err)
	}
	grpcClient := shared.NewGRPCClient(remoteConn)
	return &ClientPluginServices{
		PluginServices: shared.PluginServices{
			Store:               grpcClient,
			ArchiveStore:        grpcClient,
			StreamingSpanWriter: grpcClient,
		},
		Capabilities: grpcClient,
		remoteConn:   remoteConn,
	}, nil
}

func (c *ClientPluginServices) Close() error {
	if c.remoteConn != nil {
		return c.remoteConn.Close()
	}
	return nil
}
