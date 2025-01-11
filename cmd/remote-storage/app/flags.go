// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configgrpc"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	flagGRPCHostPort = "grpc.host-port"
)

var tlsGRPCFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "grpc",
	EnableCertReloadInterval: true,
}

// Options holds configuration for remote-storage service.
type Options struct {
	configgrpc.ServerConfig
	// Tenancy configuration
	Tenancy tenancy.Options
}

// AddFlags adds flags to flag set.
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(flagGRPCHostPort, ports.PortToHostPort(ports.RemoteStorageGRPC), "The host:port (e.g. 127.0.0.1:17271 or :17271) of the gRPC server")
	tlsGRPCFlagsConfig.AddFlags(flagSet)
	tenancy.AddFlags(flagSet)
}

// InitFromViper initializes Options with properties from CLI flags.
func (o *Options) InitFromViper(v *viper.Viper) (*Options, error) {
	o.NetAddr.Endpoint = v.GetString(flagGRPCHostPort)
	tlsGRPC, err := tlsGRPCFlagsConfig.InitFromViper(v)
	if err != nil {
		return o, fmt.Errorf("failed to process gRPC TLS options: %w", err)
	}
	o.TLSSetting = tlsGRPC
	o.Tenancy = tenancy.InitFromViper(v)
	return o, nil
}
