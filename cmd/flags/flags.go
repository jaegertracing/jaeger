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

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	hc "github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/plugin/storage"
)

const (
	spanStorageType     = "span-storage.type" // deprecated
	logLevel            = "log-level"
	configFile          = "config-file"
	healthCheckHTTPPort = "health-check-http-port"
)

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
}

type logging struct {
	Level string
}

type healthCheck struct {
	Port int
}

// AddFlags adds flags for SharedFlags
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(spanStorageType, "", fmt.Sprintf("Deprecated; please use %s environment variable", storage.SpanStorageTypeEnvVar))
	flagSet.String(logLevel, "info", "Minimal allowed log Level. For more levels see https://github.com/uber-go/zap")
	flagSet.Int(healthCheckHTTPPort, 0, "The http port for the health check service")
}

// InitFromViper initializes SharedFlags with properties from viper
func (flags *SharedFlags) InitFromViper(v *viper.Viper) *SharedFlags {
	flags.Logging.Level = v.GetString(logLevel)
	flags.HealthCheck.Port = v.GetInt(healthCheckHTTPPort)
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
func (flags *SharedFlags) NewHealthCheck(logger *zap.Logger, defaultPort int) (*hc.HealthCheck, error) {
	port := defaultPort
	if flags.HealthCheck.Port > 0 {
		port = flags.HealthCheck.Port
	}
	return hc.New(hc.Unavailable, hc.Logger(logger)).
		Serve(port)
}
