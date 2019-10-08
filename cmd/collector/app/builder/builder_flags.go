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

package builder

import (
	"flag"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	collectorQueueSize            = "collector.queue-size"
	collectorNumWorkers           = "collector.num-workers"
	collectorPort                 = "collector.port"
	collectorHTTPPort             = "collector.http-port"
	collectorGRPCPort             = "collector.grpc-port"
	collectorZipkinHTTPort        = "collector.zipkin.http-port"
	collectorZipkinAllowedOrigins = "collector.zipkin.allowed-origins"
	collectorZipkinAllowedHeaders = "collector.zipkin.allowed-headers"
)

var tlsFlagsConfig = tlscfg.FlagsConfig{
	Prefix:         "collector.grpc.",
	ShowEnabled:    true,
	ShowServerName: true,
	ShowClientCA:   true,
}

// CollectorOptions holds configuration for collector
type CollectorOptions struct {
	// QueueSize is the size of collector's queue
	QueueSize int
	// NumWorkers is the number of internal workers in a collector
	NumWorkers int
	// CollectorPort is the port that the collector service listens in on for tchannel requests
	CollectorPort int
	// CollectorHTTPPort is the port that the collector service listens in on for http requests
	CollectorHTTPPort int
	// CollectorGRPCPort is the port that the collector service listens in on for gRPC requests
	CollectorGRPCPort int
	// TLS configures secure transport
	TLS tlscfg.Options
	// CollectorZipkinHTTPPort is the port that the Zipkin collector service listens in on for http requests
	CollectorZipkinHTTPPort int
	// CollectorZipkinAllowedOrigins is a list of origins a cross-domain request to the Zipkin collector service can be executed from
	CollectorZipkinAllowedOrigins string
	// CollectorZipkinAllowedHeaders is a list of headers that the Zipkin collector service allowes the client to use with cross-domain requests
	CollectorZipkinAllowedHeaders string
}

// AddFlags adds flags for CollectorOptions
func AddFlags(flags *flag.FlagSet) {
	flags.Int(collectorQueueSize, app.DefaultQueueSize, "The queue size of the collector")
	flags.Int(collectorNumWorkers, app.DefaultNumWorkers, "The number of workers pulling items from the queue")
	flags.Int(collectorPort, ports.CollectorTChannel, "The TChannel port for the collector service")
	flags.Int(collectorHTTPPort, ports.CollectorHTTP, "The HTTP port for the collector service")
	flags.Int(collectorGRPCPort, ports.CollectorGRPC, "The gRPC port for the collector service")
	flags.Int(collectorZipkinHTTPort, 0, "The HTTP port for the Zipkin collector service e.g. 9411")
	flags.String(collectorZipkinAllowedOrigins, "*", "Comma separated list of allowed origins for the Zipkin collector service, default accepts all")
	flags.String(collectorZipkinAllowedHeaders, "content-type", "Comma separated list of allowed headers for the Zipkin collector service, default content-type")
	tlsFlagsConfig.AddFlags(flags)
}

// InitFromViper initializes CollectorOptions with properties from viper
func (cOpts *CollectorOptions) InitFromViper(v *viper.Viper) *CollectorOptions {
	cOpts.QueueSize = v.GetInt(collectorQueueSize)
	cOpts.NumWorkers = v.GetInt(collectorNumWorkers)
	cOpts.CollectorPort = v.GetInt(collectorPort)
	cOpts.CollectorHTTPPort = v.GetInt(collectorHTTPPort)
	cOpts.CollectorGRPCPort = v.GetInt(collectorGRPCPort)
	cOpts.CollectorZipkinHTTPPort = v.GetInt(collectorZipkinHTTPort)
	cOpts.CollectorZipkinAllowedOrigins = v.GetString(collectorZipkinAllowedOrigins)
	cOpts.CollectorZipkinAllowedHeaders = v.GetString(collectorZipkinAllowedHeaders)
	cOpts.TLS = tlsFlagsConfig.InitFromViper(v)
	return cOpts
}
