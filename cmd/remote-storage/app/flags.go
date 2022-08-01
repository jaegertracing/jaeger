// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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
	flagGRPCHostPort = "host-port"
)

var tlsGRPCFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix: "",
}

// Options holds configuration for query service
type Options struct {
	// GRPCHostPort is the host:port address where the service listens in on for gRPC requests
	GRPCHostPort string
	// TLSGRPC configures secure transport (Consumer to Query service GRPC API)
	TLSGRPC tlscfg.Options
	// Tenancy configures tenancy for query
	Tenancy tenancy.Options
}

// AddFlags adds flags for QueryOptions
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(flagGRPCHostPort, ports.PortToHostPort(ports.RemoteStorageGRPC), "The host:port (e.g. 127.0.0.1:17271 or :17271) of the gRPC server")
	tlsGRPCFlagsConfig.AddFlags(flagSet)
	tenancy.AddFlags(flagSet)
}

// InitFromViper initializes QueryOptions with properties from viper
func (qOpts *Options) InitFromViper(v *viper.Viper, logger *zap.Logger) (*Options, error) {
	qOpts.GRPCHostPort = v.GetString(flagGRPCHostPort)
	if tlsGrpc, err := tlsGRPCFlagsConfig.InitFromViper(v); err == nil {
		qOpts.TLSGRPC = tlsGrpc
	} else {
		return qOpts, fmt.Errorf("failed to process gRPC TLS options: %w", err)
	}
	if tenancy, err := tenancy.InitFromViper(v); err == nil {
		qOpts.Tenancy = tenancy
	} else {
		return qOpts, fmt.Errorf("failed to parse Tenancy options: %w", err)
	}
	return qOpts, nil
}
