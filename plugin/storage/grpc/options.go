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
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
)

const (
	pluginBinary             = "grpc-storage-plugin.binary"
	pluginConfigurationFile  = "grpc-storage-plugin.configuration-file"
	pluginLogLevel           = "grpc-storage-plugin.log-level"
	remotePrefix             = "grpc-storage"
	remoteServer             = remotePrefix + ".server"
	remoteConnectionTimeout  = remotePrefix + ".connection-timeout"
	defaultPluginLogLevel    = "warn"
	defaultConnectionTimeout = time.Duration(5 * time.Second)
)

// Options contains GRPC plugins configs and provides the ability
// to bind them to command line flags
type Options struct {
	Configuration config.Configuration `mapstructure:",squash"`
}

func tlsFlagsConfig() tlscfg.ClientFlagsConfig {
	return tlscfg.ClientFlagsConfig{
		Prefix: remotePrefix,
	}
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	tlsFlagsConfig().AddFlags(flagSet)

	flagSet.String(pluginBinary, "", "The location of the plugin binary")
	flagSet.String(pluginConfigurationFile, "", "A path pointing to the plugin's configuration file, made available to the plugin with the --config arg")
	flagSet.String(pluginLogLevel, defaultPluginLogLevel, "Set the log level of the plugin's logger")
	flagSet.String(remoteServer, "", "The remote storage gRPC server address")
	flagSet.Duration(remoteConnectionTimeout, defaultConnectionTimeout, "The remote storage gRPC server connection timeout")
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) error {
	opt.Configuration.PluginBinary = v.GetString(pluginBinary)
	opt.Configuration.PluginConfigurationFile = v.GetString(pluginConfigurationFile)
	opt.Configuration.PluginLogLevel = v.GetString(pluginLogLevel)
	opt.Configuration.RemoteServerAddr = v.GetString(remoteServer)
	var err error
	opt.Configuration.RemoteTLS, err = tlsFlagsConfig().InitFromViper(v)
	if err != nil {
		return fmt.Errorf("failed to parse gRPC storage TLS options: %w", err)
	}
	opt.Configuration.RemoteConnectTimeout = v.GetDuration(remoteConnectionTimeout)
	return nil
}
