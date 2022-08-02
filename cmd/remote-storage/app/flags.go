// Copyright (c) 2022 The Jaeger Authors.
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

package app

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	flagGRPCHostPort = "grpc.host-port"
)

var tlsGRPCFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "grpc",
}

// Options holds configuration for remote-storage service.
type Options struct {
	// GRPCHostPort is the host:port address for gRPC server
	GRPCHostPort string
	// TLSGRPC configures secure transport
	TLSGRPC tlscfg.Options
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
func (o *Options) InitFromViper(v *viper.Viper, logger *zap.Logger) (*Options, error) {
	o.GRPCHostPort = v.GetString(flagGRPCHostPort)
	if tlsGrpc, err := tlsGRPCFlagsConfig.InitFromViper(v); err == nil {
		o.TLSGRPC = tlsGrpc
	} else {
		return o, fmt.Errorf("failed to process gRPC TLS options: %w", err)
	}
	o.Tenancy = tenancy.InitFromViper(v)
	return o, nil
}
