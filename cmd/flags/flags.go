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
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	// CassandraStorageType is the storage type flag denoting a Cassandra backing store
	CassandraStorageType = "cassandra"
	// MemoryStorageType is the storage type flag denoting an in-memory store
	MemoryStorageType = "memory"
	// ESStorageType is the storage type flag denoting an ElasticSearch backing store
	ESStorageType                  = "elasticsearch"
	spanStorageType                = "span-storage.type"
	logLevel                       = "log-Level"
	dependencyStorageDataFrequency = "dependency-storage.data-frequency"
	configFile                     = "config-file"
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
			errors.Wrap(err, fmt.Sprintf("Error loading config file %s", file))
		}
	}
	return nil
}

// NewLogger pareses log Level
func (flags *SharedFlags) NewLogger(conf zap.Config, options ...zap.Option) (*zap.Logger, error) {
	var level zapcore.Level
	err := (&level).UnmarshalText([]byte(flags.Logging.Level))
	if err != nil {
		return nil, err
	}
	conf.Level = zap.NewAtomicLevelAt(level)
	return conf.Build(options...)
}

// SharedFlags holds flags configuration
type SharedFlags struct {
	// SpanStorage defines common settings for Span Storage.
	SpanStorage spanStorage
	// DependencyStorage defines common settings for Dependency Storage.
	DependencyStorage dependencyStorage
	// Logging holds logging configuration
	Logging logging
}

// InitFromViper initializes SharedFlags with properties from viper
func (flags *SharedFlags) InitFromViper(v *viper.Viper) *SharedFlags {
	flags.SpanStorage.Type = v.GetString(spanStorageType)
	flags.DependencyStorage.DataFrequency = v.GetDuration(dependencyStorageDataFrequency)
	flags.Logging.Level = v.GetString(logLevel)
	return flags
}

// AddFlags adds flags for SharedFlags
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(spanStorageType, CassandraStorageType, fmt.Sprintf("The type of span storage backend to use, options are currently [%v,%v,%v]", CassandraStorageType, ESStorageType, MemoryStorageType))
	flagSet.String(logLevel, "info", "Minimal allowed log Level. For more levels see zap logger documentation")
	flagSet.Duration(dependencyStorageDataFrequency, time.Hour*24, "Frequency of service dependency calculations")
}

// ErrUnsupportedStorageType is the error when dealing with an unsupported storage type
var ErrUnsupportedStorageType = errors.New("Storage Type is not supported")

type logging struct {
	Level string
}

type spanStorage struct {
	Type string
}

type dependencyStorage struct {
	DataFrequency time.Duration
}
