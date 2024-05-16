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

	v2.ClientConfig.TLSSetting.Insecure = !c.RemoteTLS.Enabled

	v2.ClientConfig.TLSSetting.ServerName = c.RemoteTLS.ServerName
	v2.ClientConfig.TLSSetting.InsecureSkipVerify = c.RemoteTLS.SkipHostVerify
	v2.ClientConfig.TLSSetting.CAFile = c.RemoteTLS.CAPath
	v2.ClientConfig.TLSSetting.CertFile = c.RemoteTLS.CertPath
	v2.ClientConfig.TLSSetting.KeyFile = c.RemoteTLS.KeyPath
	v2.ClientConfig.TLSSetting.CipherSuites = c.RemoteTLS.CipherSuites
	v2.ClientConfig.TLSSetting.MinVersion = c.RemoteTLS.MinVersion
	v2.ClientConfig.TLSSetting.MaxVersion = c.RemoteTLS.MaxVersion
	v2.ClientConfig.TLSSetting.ReloadInterval = c.RemoteTLS.ReloadInterval

	return v2
}

// ClientPluginServices defines services plugin can expose and its capabilities
type ClientPluginServices struct {
	shared.PluginServices
	Capabilities shared.PluginCapabilities
}

// PluginBuilder is used to create storage plugins. Implemented by Configuration.
// TODO this interface should be removed and the building capability moved to Factory.
type PluginBuilder interface {
	Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, *grpc.ClientConn, error)
}

// Build instantiates a PluginServices
func (c *Configuration) Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, *grpc.ClientConn, error) {
	v2Cfg := c.translateToConfigV2()
	s, remoteConn, err := newRemoteStorage(v2Cfg, tracerProvider)
	if err != nil {
		return nil, nil, err
	}
	return s, remoteConn, nil
}

func (c *ConfigV2) Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, *grpc.ClientConn, error) {
	s, remoteConn, err := newRemoteStorage(c, tracerProvider)
	if err != nil {
		return nil, nil, err
	}
	return s, remoteConn, nil
}

func newRemoteStorage(c *ConfigV2, tracerProvider trace.TracerProvider) (*ClientPluginServices, *grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tracerProvider))),
	}
	if c.Auth != nil {
		return nil, nil, fmt.Errorf("authenticator is not supported")
	}

	tenancyMgr := tenancy.NewManager(&c.TenancyOpts)
	if tenancyMgr.Enabled {
		opts = append(opts, grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tenancyMgr)))
		opts = append(opts, grpc.WithStreamInterceptor(tenancy.NewClientStreamInterceptor(tenancyMgr)))
	}

	remoteConn, err := c.ToClientConn(context.Background(), componenttest.NewNopHost(), component.TelemetrySettings{}, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating remote storage client: %w", err)
	}
	grpcClient := shared.NewGRPCClient(remoteConn)
	return &ClientPluginServices{
		PluginServices: shared.PluginServices{
			Store:               grpcClient,
			ArchiveStore:        grpcClient,
			StreamingSpanWriter: grpcClient,
		},
		Capabilities: grpcClient,
	}, remoteConn, nil
}
