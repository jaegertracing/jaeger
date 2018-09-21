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

package flags

import (
	"flag"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	hc "github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/plugin/pkg/factory"
	"github.com/jaegertracing/jaeger/plugin/storage"
)

const (
	spanStorageType     = "span-storage.type" // deprecated
	logLevel            = "log-level"
	configFile          = "config-file"
	healthCheckHTTPPort = "health-check-http-port"
	pluginDirectory     = "plugin-directory"
)

var defaultHealthCheckPort int

// AddConfigFileFlag adds flags for ExternalConfFlags
func AddConfigFileFlag(flagSet *flag.FlagSet) {
	flagSet.String(configFile, "", "Configuration file in JSON, TOML, YAML, HCL, or Java properties formats (default none). See spf13/viper for precedence.")
}

// TryLoadConfigFile initializes viper with config file specified as flag
func TryLoadConfigFile(v *viper.Viper) error {
	if file := v.GetString(configFile); file != "" {
		v.SetConfigFile(file)
		err := v.ReadInConfig()
		if err != nil {
			return errors.Wrapf(err, "Error loading config file %s", file)
		}
	}
	return nil
}

// SharedFlags holds flags configuration
type SharedFlags struct {
	// Logging holds logging configuration
	Logging logging
	// HealthCheck holds health check configuration
	HealthCheck healthCheck
	// Plugin holds plugin configuration
	Plugin plugin
}

type logging struct {
	Level string
}

type healthCheck struct {
	Port int
}

type plugin struct {
	Directory string
}

// SetDefaultHealthCheckPort sets the default port for health check. Must be called before AddFlags
func SetDefaultHealthCheckPort(port int) {
	defaultHealthCheckPort = port
}

// AddFlags adds flags for SharedFlags
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(spanStorageType, "", fmt.Sprintf(`Deprecated; please use %s environment variable. Run this binary with "env" command for help.`, storage.SpanStorageTypeEnvVar))
	AddHealthcheckFlag(flagSet)
	AddLoggingFlag(flagSet)
	AddPluginFlag(flagSet)
}

// AddHealthcheckFlag adds logging flag for SharedFlags
func AddHealthcheckFlag(flagSet *flag.FlagSet) {
	flagSet.Int(healthCheckHTTPPort, defaultHealthCheckPort, "The http port for the health check service")
}

// AddLoggingFlag adds logging flag for SharedFlags
func AddLoggingFlag(flagSet *flag.FlagSet) {
	flagSet.String(logLevel, "info", "Minimal allowed log Level. For more levels see https://github.com/uber-go/zap")
}

// AddPluginFlag adds plugin flag for SharedFlags
func AddPluginFlag(flagSet *flag.FlagSet) {
	flagSet.String(pluginDirectory, "", "The directory to dynamically load plugins")
}

// InitFromViper initializes SharedFlags with properties from viper
func (flags *SharedFlags) InitFromViper(v *viper.Viper) *SharedFlags {
	flags.Logging.Level = v.GetString(logLevel)
	flags.HealthCheck.Port = v.GetInt(healthCheckHTTPPort)
	flags.Plugin.Directory = v.GetString(pluginDirectory)
	return flags
}

// NewLogger returns logger based on configuration in SharedFlags
func (flags *SharedFlags) NewLogger(conf zap.Config, options ...zap.Option) (*zap.Logger, error) {
	var level zapcore.Level
	err := (&level).UnmarshalText([]byte(flags.Logging.Level))
	if err != nil {
		return nil, err
	}
	conf.Level = zap.NewAtomicLevelAt(level)
	return conf.Build(options...)
}

// NewHealthCheck returns health check based on configuration in SharedFlags
func (flags *SharedFlags) NewHealthCheck(logger *zap.Logger) (*hc.HealthCheck, error) {
	if flags.HealthCheck.Port == 0 {
		return nil, errors.New("port not specified")
	}
	return hc.New(hc.Unavailable, hc.Logger(logger)).
		Serve(flags.HealthCheck.Port)
}

// NewPluginFactory returns plugin factory based on configuration in SharedFlags
func (flags *SharedFlags) NewPluginFactory(logger *zap.Logger) (factory.PluginFactory, error) {
	if flags.Plugin.Directory != "" {
		fi, err := os.Stat(flags.Plugin.Directory)
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			return nil, errors.Errorf("The provided plugin directory (%s) is not a directory", flags.Plugin.Directory)
		}
	}
	return factory.NewPluginFactory(flags.Plugin.Directory, logger), nil
}
