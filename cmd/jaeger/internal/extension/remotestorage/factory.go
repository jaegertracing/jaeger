// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotestorage

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
	"github.com/jaegertracing/jaeger/ports"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("remote_storage")

// ID is the identifier of this extension.
var ID = component.NewID(componentType)

func NewFactory() extension.Factory {
	return extension.NewFactory(
		componentType,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelDevelopment,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Options: app.Options{
			ServerConfig: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ports.PortToHostPort(ports.RemoteStorageGRPC),
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
	}
}

func createExtension(
	_ context.Context,
	set extension.Settings,
	cfg component.Config,
) (extension.Extension, error) {
	return newServer(cfg.(*Config), set.TelemetrySettings), nil
}
