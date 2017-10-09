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
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	// CassandraStorageType is the storage type flag denoting a Cassandra backing store
	CassandraStorageType = "cassandra"
	// MemoryStorageType is the storage type flag denoting an in-memory store
	MemoryStorageType = "memory"
	// ESStorageType is the storage type flag denoting an ElasticSearch backing store
	ESStorageType                  = "elasticsearch"
	runtimeMetricsFrequency        = "runtime-metrics-frequency"
	spanStorageType                = "span-storage.type"
	logLevel                       = "log-level"
	dependencyStorageType          = "dependency-storage.type"
	dependencyStorageDataFrequency = "dependency-storage.data-frequency"
	configFile                     = "config-file"
)

// ExternalConfFlags holds configuration for external sources like configuration files
type ExternalConfFlags struct {
	ConfigFile string
}

// AddConfFileFlag adds flags for ExternalConfFlags
func AddConfFileFlag(flagSet *flag.FlagSet) {
	flagSet.String(configFile, "", "Configuration file in JSON, TOML, YAML, HCL, or Java properties formats (default none). See spf13/viper for precedence.")
}

// InitFromViper initializes ExternalConfFlags with properties from viper
func (flags *ExternalConfFlags) InitFromViper(v *viper.Viper) *ExternalConfFlags {
	flags.ConfigFile = v.GetString(configFile)
	return flags
}

// SharedFlags holds flags configuration
type SharedFlags struct {
	// SpanStorage defines common settings for Span Storage.
	SpanStorage spanStorage
	// DependencyStorage defines common settings for Dependency Storage.
	DependencyStorage dependencyStorage
}

// InitFromViper initializes SharedFlags with properties from viper
func (flags *SharedFlags) InitFromViper(v *viper.Viper) *SharedFlags {
	flags.SpanStorage.Type = v.GetString(spanStorageType)
	flags.DependencyStorage.Type = v.GetString(dependencyStorageType)
	flags.DependencyStorage.DataFrequency = v.GetDuration(dependencyStorageDataFrequency)
	return flags
}

// AddFlags adds flags for SharedFlags
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Duration(runtimeMetricsFrequency, 1*time.Second, "The frequency of reporting Go runtime metrics")
	flagSet.String(spanStorageType, CassandraStorageType, fmt.Sprintf("The type of span storage backend to use, options are currently [%v,%v,%v]", CassandraStorageType, ESStorageType, MemoryStorageType))
	flagSet.String(logLevel, "info", "Minimal allowed log level")
	flagSet.String(dependencyStorageType, CassandraStorageType, fmt.Sprintf("The type of dependency storage backend to use, options are currently [%v,%v,%v]", CassandraStorageType, ESStorageType, MemoryStorageType))
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
	Type          string
	DataFrequency time.Duration
}

type cassandraOptions struct {
	ConnectionsPerHost int
	MaxRetryAttempts   int
	QueryTimeout       time.Duration
	Servers            string
	Port               int
	Keyspace           string
}

func (co cassandraOptions) ServerList() []string {
	return strings.Split(co.Servers, ",")
}
