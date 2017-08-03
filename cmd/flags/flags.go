// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
)

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
	flagSet.String(spanStorageType, CassandraStorageType, fmt.Sprintf("The type of span storage backend to use, options are currently [%v,%v]", CassandraStorageType, MemoryStorageType))
	flagSet.String(logLevel, "info", "Minimal allowed log level")
	flagSet.String(dependencyStorageType, CassandraStorageType, fmt.Sprintf("The type of dependency storage backend to use, options are currently [%v,%v]", CassandraStorageType, MemoryStorageType))
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
