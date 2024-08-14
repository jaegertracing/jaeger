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

package grpc

import (
	"flag"
	"fmt"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

const (
	remotePrefix             = "grpc-storage"
	remoteServer             = remotePrefix + ".server"
	remoteConnectionTimeout  = remotePrefix + ".connection-timeout"
	defaultConnectionTimeout = time.Duration(5 * time.Second)
)

func tlsFlagsConfig() tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: remotePrefix,
	}
}

// AddFlags adds flags for Options
func v1AddFlags(flagSet *flag.FlagSet) {
	tlsFlagsConfig().AddFlags(flagSet)

	flagSet.String(remoteServer, "", "The remote storage gRPC server address as host:port")
	flagSet.Duration(remoteConnectionTimeout, defaultConnectionTimeout, "The remote storage gRPC server connection timeout")
}

func v1InitFromViper(cfg *Configuration, v *viper.Viper) error {
	cfg.RemoteServerAddr = v.GetString(remoteServer)
	var err error
	cfg.RemoteTLS, err = tlsFlagsConfig().InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse gRPC storage TLS options: %w", err)
	}
	cfg.RemoteConnectTimeout = v.GetDuration(remoteConnectionTimeout)
	cfg.TenancyOpts = tenancy.InitFromViper(v)
	return nil
}
