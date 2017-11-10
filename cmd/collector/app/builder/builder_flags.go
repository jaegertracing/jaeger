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
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/cmd/collector/app"
)

const (
	collectorQueueSize           = "collector.queue-size"
	collectorNumWorkers          = "collector.num-workers"
	collectorWriteCacheTTL       = "collector.write-cache-ttl"
	collectorPort                = "collector.port"
	collectorHTTPPort            = "collector.http-port"
	collectorZipkinHTTPort       = "collector.zipkin.http-port"
	collectorHealthCheckHTTPPort = "collector.health-check-http-port"
)

// CollectorOptions holds configuration for collector
type CollectorOptions struct {
	// QueueSize is the size of collector's queue
	QueueSize int
	// NumWorkers is the number of internal workers in a collector
	NumWorkers int
	// WriteCacheTTL denotes how often to check and re-write a service or operation name
	WriteCacheTTL time.Duration
	// CollectorPort is the port that the collector service listens in on for tchannel requests
	CollectorPort int
	// CollectorHTTPPort is the port that the collector service listens in on for http requests
	CollectorHTTPPort int
	// CollectorZipkinHTTPPort is the port that the Zipkin collector service listens in on for http requests
	CollectorZipkinHTTPPort int
	// CollectorHealthCheckHTTPPort is the port that the health check service listens in on for http requests
	CollectorHealthCheckHTTPPort int
}

// AddFlags adds flags for CollectorOptions
func AddFlags(flags *flag.FlagSet) {
	flags.Int(collectorQueueSize, app.DefaultQueueSize, "The queue size of the collector")
	flags.Int(collectorNumWorkers, app.DefaultNumWorkers, "The number of workers pulling items from the queue")
	flags.Duration(collectorWriteCacheTTL, time.Hour*12, "The duration to wait before rewriting an existing service or operation name")
	flags.Int(collectorPort, 14267, "The tchannel port for the collector service")
	flags.Int(collectorHTTPPort, 14268, "The http port for the collector service")
	flags.Int(collectorZipkinHTTPort, 0, "The http port for the Zipkin collector service e.g. 9411")
	flags.Int(collectorHealthCheckHTTPPort, 14269, "The http port for the health check service")
}

// InitFromViper initializes CollectorOptions with properties from viper
func (cOpts *CollectorOptions) InitFromViper(v *viper.Viper) *CollectorOptions {
	cOpts.QueueSize = v.GetInt(collectorQueueSize)
	cOpts.NumWorkers = v.GetInt(collectorNumWorkers)
	cOpts.WriteCacheTTL = v.GetDuration(collectorWriteCacheTTL)
	cOpts.CollectorPort = v.GetInt(collectorPort)
	cOpts.CollectorHTTPPort = v.GetInt(collectorHTTPPort)
	cOpts.CollectorZipkinHTTPPort = v.GetInt(collectorZipkinHTTPort)
	cOpts.CollectorHealthCheckHTTPPort = v.GetInt(collectorHealthCheckHTTPPort)
	return cOpts
}
