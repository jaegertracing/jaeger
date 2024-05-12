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
	"errors"
	"fmt"
	"time"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

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

	remoteConn *grpc.ClientConn
}

type ConfigV2 struct {
	Configuration `mapstructure:",squash"`

	configgrpc.ClientConfig        `mapstructure:",squash"`
	exporterhelper.TimeoutSettings `mapstructure:",squash"`
}

func translateToConfigV2(v1 Configuration) *ConfigV2 {
	return &ConfigV2{
		Configuration: v1,

		ClientConfig: configgrpc.ClientConfig{
			Endpoint: v1.RemoteServerAddr,

			TLSSetting: configtls.ClientConfig{
				Config: configtls.Config{
					CAFile:         v1.RemoteTLS.CAPath,
					CertFile:       v1.RemoteTLS.CertPath,
					KeyFile:        v1.RemoteTLS.KeyPath,
					CipherSuites:   v1.RemoteTLS.CipherSuites,
					MinVersion:     v1.RemoteTLS.MinVersion,
					MaxVersion:     v1.RemoteTLS.MaxVersion,
					ReloadInterval: v1.RemoteTLS.ReloadInterval,
				},

				ServerName:         v1.RemoteTLS.ServerName,
				InsecureSkipVerify: v1.RemoteTLS.SkipHostVerify,
			},
		},

		TimeoutSettings: exporterhelper.TimeoutSettings{
			Timeout: v1.RemoteConnectTimeout,
		},
	}

}

// ClientPluginServices defines services plugin can expose and its capabilities
type ClientPluginServices struct {
	shared.PluginServices
	Capabilities shared.PluginCapabilities
}

// PluginBuilder is used to create storage plugins. Implemented by Configuration.
// TODO this interface should be removed and the building capability moved to Factory.
type PluginBuilder interface {
	Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error)
	Close() error
}

// Build instantiates a PluginServices
func (c *Configuration) Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error) {
	return c.buildRemote(logger, tracerProvider, grpc.NewClient)
}

type newClientFn func(target string, opts ...grpc.DialOption) (conn *grpc.ClientConn, err error)

func (c *Configuration) buildRemote(logger *zap.Logger, tracerProvider trace.TracerProvider, newClient newClientFn) (*ClientPluginServices, error) {
c = translateToConfigV2(c.Configuration)
	opts := []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tracerProvider))),
		grpc.WithBlock(),
	}

	if c.Auth != nil {
		return nil, fmt.Errorf("authenticator is not supported")
	}

	if c.RemoteTLS.Enabled {
		tlsCfg, err := c.RemoteTLS.Config(logger)
		if err != nil {
			return nil, err
		}
		creds := credentials.NewTLS(tlsCfg)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	tenancyMgr := tenancy.NewManager(&c.TenancyOpts)
	if tenancyMgr.Enabled {
		opts = append(opts, grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tenancyMgr)))
		opts = append(opts, grpc.WithStreamInterceptor(tenancy.NewClientStreamInterceptor(tenancyMgr)))
	}

	var err error
	c.remoteConn, err = c.ToClientConn(ctx, componenttest.NewNopHost(), component.TelemetrySettings{}, opts...)
	c.remoteConn, err = newClient(c.RemoteServerAddr, opts...)

	if err != nil {
		return nil, fmt.Errorf("error creating remote storage client: %w", err)
	}

	grpcClient := shared.NewGRPCClient(c.remoteConn)
	return &ClientPluginServices{
		PluginServices: shared.PluginServices{
			Store:               grpcClient,
			ArchiveStore:        grpcClient,
			StreamingSpanWriter: grpcClient,
		},
		Capabilities: grpcClient,
	}, nil
}

func (c *Configuration) Close() error {
	var errs []error
	if c.remoteConn != nil {
		errs = append(errs, c.remoteConn.Close())
	}
	errs = append(errs, c.RemoteTLS.Close())
	return errors.Join(errs...)
}
