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

package config

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
	TenancyOpts                    tenancy.Options
	configgrpc.ClientConfig        `mapstructure:",squash"`
	exporterhelper.TimeoutSettings `mapstructure:",squash"`
}

func (c *Configuration) translateToConfigV2() *ConfigV2 {
	v2 := &ConfigV2{}
	v2.Endpoint = c.RemoteServerAddr
	v2.Timeout = c.RemoteConnectTimeout
	v2.TLSSetting = c.RemoteTLS.ToOtelClientConfig(&v2.TLSSetting)

	return v2
}

// ClientPluginServices defines services plugin can expose and its capabilities
type ClientPluginServices struct {
	shared.PluginServices
	Capabilities shared.PluginCapabilities
	remoteConn   *grpc.ClientConn
}

// PluginBuilder is used to create storage plugins. Implemented by Configuration.
// TODO this interface should be removed and the building capability moved to Factory.
type PluginBuilder interface {
	Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error)
}

// Build instantiates a PluginServices
func (c *Configuration) Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error) {
	v2Cfg := c.translateToConfigV2()
	return v2Cfg.Build(logger, tracerProvider)
}

func (c *ConfigV2) Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error) {
	s, err := newRemoteStorage(c, tracerProvider)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func newRemoteStorage(c *ConfigV2, tracerProvider trace.TracerProvider) (*ClientPluginServices, error) {
	opts := []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tracerProvider))),
	}
	if c.Auth != nil {
		return nil, fmt.Errorf("authenticator is not supported")
	}

	tenancyMgr := tenancy.NewManager(&c.TenancyOpts)
	if tenancyMgr.Enabled {
		opts = append(opts, grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tenancyMgr)))
		opts = append(opts, grpc.WithStreamInterceptor(tenancy.NewClientStreamInterceptor(tenancyMgr)))
	}

	remoteConn, err := c.ToClientConn(context.Background(), componenttest.NewNopHost(), component.TelemetrySettings{}, opts...)
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
		err := c.remoteConn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
