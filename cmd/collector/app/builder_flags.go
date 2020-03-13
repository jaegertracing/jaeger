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

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/flags"
	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/ports"
)

const (
	collectorDynQueueSizeMemory   = "collector.queue-size-memory"
	collectorQueueSize            = "collector.queue-size"
	collectorNumWorkers           = "collector.num-workers"
	collectorHTTPPort             = "collector.http-port"
	collectorGRPCPort             = "collector.grpc-port"
	collectorTChanHostPort        = "collector.tchan-server.host-port"
	collectorHTTPHostPort         = "collector.http-server.host-port"
	collectorGRPCHostPort         = "collector.grpc-server.host-port"
	collectorZipkinHTTPPort       = "collector.zipkin.http-port"
	collectorZipkinHTTPHostPort   = "collector.zipkin.host-port"
	collectorTags                 = "collector.tags"
	collectorZipkinAllowedOrigins = "collector.zipkin.allowed-origins"
	collectorZipkinAllowedHeaders = "collector.zipkin.allowed-headers"
)

var tlsFlagsConfig = tlscfg.ServerFlagsConfig{
	Prefix:       "collector.grpc",
	ShowEnabled:  true,
	ShowClientCA: true,
}

// CollectorOptions holds configuration for collector
type CollectorOptions struct {
	// DynQueueSizeMemory determines how much memory to use for the queue
	DynQueueSizeMemory uint
	// QueueSize is the size of collector's queue
	QueueSize int
	// NumWorkers is the number of internal workers in a collector
	NumWorkers int
	// CollectorHTTPPort is the port that the collector service listens in on for http requests
	CollectorHTTPPort int
	// CollectorGRPCPort is the port that the collector service listens in on for gRPC requests
	CollectorGRPCPort int
	// CollectorTChanHostPort is the host:port address that the collector service listens in on for tchannel requests
	CollectorTChanHostPort string
	// CollectorHTTPHostPort is the host:port address that the collector service listens in on for http requests
	CollectorHTTPHostPort string
	// CollectorGRPCHostPort is the host:port address that the collector service listens in on for gRPC requests
	CollectorGRPCHostPort string
	// TLS configures secure transport
	TLS tlscfg.Options
	// CollectorTags is the string representing collector tags to append to each and every span
	CollectorTags map[string]string
	// CollectorZipkinHTTPPort is the port that the Zipkin collector service listens in on for http requests
	CollectorZipkinHTTPPort int
	// CollectorZipkinHTTPHostPort is the host:port address that the Zipkin collector service listens in on for http requests
	CollectorZipkinHTTPHostPort string
	// CollectorZipkinAllowedOrigins is a list of origins a cross-domain request to the Zipkin collector service can be executed from
	CollectorZipkinAllowedOrigins string
	// CollectorZipkinAllowedHeaders is a list of headers that the Zipkin collector service allowes the client to use with cross-domain requests
	CollectorZipkinAllowedHeaders string
}

// AddFlags adds flags for CollectorOptions
func AddFlags(flags *flag.FlagSet) {
	flags.Int(collectorQueueSize, app.DefaultQueueSize, "The queue size of the collector")
	flags.Int(collectorNumWorkers, app.DefaultNumWorkers, "The number of workers pulling items from the queue")
	flags.Int(collectorPort, 0, "(deprecated) please use - "+collectorTChanHostPort)
	flags.Int(collectorHTTPPort, 0, "(deprecated) please use -"+collectorHTTPHostPort)
	flags.Int(collectorGRPCPort, 0, "(deprecated) please use -"+collectorGRPCHostPort)
	flags.Int(collectorZipkinHTTPPort, 0, "(deprecated) please use -"+collectorZipkinHTTPHostPort)
	flags.String(collectorTChanHostPort, ports.PortToHostPort(ports.CollectorTChannel), "The host:port (e.g. 127.0.0.1:5555 or :5555) of the collector's TChannel server")
	flags.String(collectorHTTPHostPort, ports.PortToHostPort(ports.CollectorHTTP), "The host:port (e.g. 127.0.0.1:5555 or :5555) of the collector's HTTP server")
	flags.String(collectorGRPCHostPort, ports.PortToHostPort(ports.CollectorGRPC), "The host:port (e.g. 127.0.0.1:5555 or :5555) of the collector's GRPC server")
	flags.String(collectorZipkinHTTPHostPort, ports.PortToHostPort(0), "The host:port (e.g. 127.0.0.1:5555 or :5555) of the collector's Zipkin server")
	flags.Uint(collectorDynQueueSizeMemory, 0, "(experimental) The max memory size in MiB to use for the dynamic queue.")
	flags.String(collectorTags, "", "One or more tags to be added to the Process tags of all spans passing through this collector. Ex: key1=value1,key2=${envVar:defaultValue}")
	flags.String(collectorZipkinAllowedOrigins, "*", "Comma separated list of allowed origins for the Zipkin collector service, default accepts all")
	flags.String(collectorZipkinAllowedHeaders, "content-type", "Comma separated list of allowed headers for the Zipkin collector service, default content-type")
	tlsFlagsConfig.AddFlags(flags)
}

// InitFromViper initializes CollectorOptions with properties from viper
func (cOpts *CollectorOptions) InitFromViper(v *viper.Viper) *CollectorOptions {
	cOpts.DynQueueSizeMemory = v.GetUint(collectorDynQueueSizeMemory) * 1024 * 1024 // we receive in MiB and store in bytes
	cOpts.QueueSize = v.GetInt(collectorQueueSize)
	cOpts.NumWorkers = v.GetInt(collectorNumWorkers)
	cOpts.CollectorHTTPPort = v.GetInt(collectorHTTPPort)
	cOpts.CollectorGRPCPort = v.GetInt(collectorGRPCPort)
	cOpts.CollectorTChanHostPort = v.GetString(collectorTChanHostPort)
	cOpts.CollectorHTTPHostPort = v.GetString(collectorHTTPHostPort)
	cOpts.CollectorGRPCHostPort = v.GetString(collectorGRPCHostPort)
	cOpts.CollectorZipkinHTTPPort = v.GetInt(collectorZipkinHTTPPort)
	cOpts.CollectorZipkinHTTPHostPort = v.GetString(collectorZipkinHTTPHostPort)
	cOpts.CollectorTags = flags.ParseJaegerTags(v.GetString(collectorTags))
	cOpts.CollectorZipkinAllowedOrigins = v.GetString(collectorZipkinAllowedOrigins)
	cOpts.CollectorZipkinAllowedHeaders = v.GetString(collectorZipkinAllowedHeaders)
	cOpts.TLS = tlsFlagsConfig.InitFromViper(v)
	return cOpts
}
