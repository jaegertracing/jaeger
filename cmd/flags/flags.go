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

package flags

import (
	"flag"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	spanStorageType = "span-storage.type" // deprecated
	logLevel        = "log-level"
	configFile      = "config-file"
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
			return errors.Wrapf(err, "cannot load config file %s", file)
		}
	}
	return nil
}

// SharedFlags holds flags configuration
type SharedFlags struct {
	// Logging holds logging configuration
	Logging logging
}

type logging struct {
	Level string
}

// AddFlags adds flags for SharedFlags
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(spanStorageType, "", "(deprecated) please use SPAN_STORAGE_TYPE environment variable. Run this binary with the 'env' command for help.")
	AddLoggingFlag(flagSet)
}

// AddLoggingFlag adds logging flag for SharedFlags
func AddLoggingFlag(flagSet *flag.FlagSet) {
	flagSet.String(logLevel, "info", "Minimal allowed log Level. For more levels see https://github.com/uber-go/zap")
}

// InitFromViper initializes SharedFlags with properties from viper
func (flags *SharedFlags) InitFromViper(v *viper.Viper) *SharedFlags {
	flags.Logging.Level = v.GetString(logLevel)
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
