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

package config

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
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

var pluginHealthCheckInterval = time.Second * 60

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	PluginBinary            string `yaml:"binary" mapstructure:"binary"`
	PluginConfigurationFile string `yaml:"configuration-file" mapstructure:"configuration_file"`
	PluginLogLevel          string `yaml:"log-level" mapstructure:"log_level"`
	RemoteServerAddr        string `yaml:"server" mapstructure:"server"`
	RemoteTLS               tlscfg.Options
	RemoteConnectTimeout    time.Duration `yaml:"connection-timeout" mapstructure:"connection-timeout"`
	TenancyOpts             tenancy.Options

	pluginHealthCheck     *time.Ticker
	pluginHealthCheckDone chan bool
	pluginRPCClient       plugin.ClientProtocol
	remoteConn            *grpc.ClientConn
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
	Capabilities     shared.PluginCapabilities
	killPluginClient func()
}

func (c *ClientPluginServices) Close() error {
	if c.killPluginClient != nil {
		c.killPluginClient()
	}
	return nil
}

// PluginBuilder is used to create storage plugins. Implemented by Configuration.
type PluginBuilder interface {
	Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error)
	Close() error
}

// Build instantiates a PluginServices
func (c *ConfigV2) Build(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error) {
	if c.PluginBinary != "" {
		return c.buildPlugin(logger, tracerProvider)
	} else {
		return c.buildRemote(logger, tracerProvider)
	}
}

func (c *Configuration) Close() error {
	if c.pluginHealthCheck != nil {
		c.pluginHealthCheck.Stop()
		c.pluginHealthCheckDone <- true
	}
	if c.remoteConn != nil {
		c.remoteConn.Close()
	}

	return c.RemoteTLS.Close()
}

func (c *ConfigV2) buildRemote(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error) {
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

	ctx, cancel := context.WithTimeout(context.Background(), c.RemoteConnectTimeout)
	defer cancel()

	tenancyMgr := tenancy.NewManager(&c.TenancyOpts)
	if tenancyMgr.Enabled {
		opts = append(opts, grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tenancyMgr)))
		opts = append(opts, grpc.WithStreamInterceptor(tenancy.NewClientStreamInterceptor(tenancyMgr)))
	}

	var err error
	c.remoteConn, err = c.ToClientConn(ctx, componenttest.NewNopHost(), component.TelemetrySettings{}, opts...)
	if err != nil {
		return nil, fmt.Errorf("error connecting to remote storage: %w", err)
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

func (c *ConfigV2) buildPlugin(logger *zap.Logger, tracerProvider trace.TracerProvider) (*ClientPluginServices, error) {
	opts := []grpc.DialOption{
		grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tracerProvider))),
	}

	tenancyMgr := tenancy.NewManager(&c.TenancyOpts)
	if tenancyMgr.Enabled {
		opts = append(opts, grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tenancyMgr)))
		opts = append(opts, grpc.WithStreamInterceptor(tenancy.NewClientStreamInterceptor(tenancyMgr)))
	}

	// #nosec G204
	cmd := exec.Command(c.PluginBinary, "--config", c.PluginConfigurationFile)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: shared.Handshake,
		VersionedPlugins: map[int]plugin.PluginSet{
			1: shared.PluginMap,
		},
		Cmd:              cmd,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Logger: hclog.New(&hclog.LoggerOptions{
			Level: hclog.LevelFromString(c.PluginLogLevel),
		}),
		GRPCDialOptions: opts,
	})

	rpcClient, err := client.Client()
	if err != nil {
		return nil, fmt.Errorf("error attempting to connect to plugin rpc client: %w", err)
	}

	raw, err := rpcClient.Dispense(shared.StoragePluginIdentifier)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve storage plugin instance: %w", err)
	}

	// in practice, the type of `raw` is *shared.grpcClient, and type casts below cannot fail
	storagePlugin, ok := raw.(shared.StoragePlugin)
	if !ok {
		return nil, fmt.Errorf("unable to cast %T to shared.StoragePlugin for plugin \"%s\"",
			raw, shared.StoragePluginIdentifier)
	}
	archiveStoragePlugin, ok := raw.(shared.ArchiveStoragePlugin)
	if !ok {
		return nil, fmt.Errorf("unable to cast %T to shared.ArchiveStoragePlugin for plugin \"%s\"",
			raw, shared.StoragePluginIdentifier)
	}
	streamingSpanWriterPlugin, ok := raw.(shared.StreamingSpanWriterPlugin)
	if !ok {
		return nil, fmt.Errorf("unable to cast %T to shared.StreamingSpanWriterPlugin for plugin \"%s\"",
			raw, shared.StoragePluginIdentifier)
	}
	capabilities, ok := raw.(shared.PluginCapabilities)
	if !ok {
		return nil, fmt.Errorf("unable to cast %T to shared.PluginCapabilities for plugin \"%s\"",
			raw, shared.StoragePluginIdentifier)
	}

	if err := c.startPluginHealthCheck(rpcClient, logger); err != nil {
		return nil, fmt.Errorf("initial plugin health check failed: %w", err)
	}

	return &ClientPluginServices{
		PluginServices: shared.PluginServices{
			Store:               storagePlugin,
			ArchiveStore:        archiveStoragePlugin,
			StreamingSpanWriter: streamingSpanWriterPlugin,
		},
		Capabilities:     capabilities,
		killPluginClient: client.Kill,
	}, nil
}

func (c *ConfigV2) startPluginHealthCheck(rpcClient plugin.ClientProtocol, logger *zap.Logger) error {
	c.pluginRPCClient = rpcClient
	c.pluginHealthCheckDone = make(chan bool)
	c.pluginHealthCheck = time.NewTicker(pluginHealthCheckInterval)

	go func() {
		for {
			select {
			case <-c.pluginHealthCheckDone:
				return
			case <-c.pluginHealthCheck.C:
				if err := c.pluginRPCClient.Ping(); err != nil {
					logger.Fatal("plugin health check failed", zap.Error(err))
				}
			}
		}
	}()

	return c.pluginRPCClient.Ping()
}
