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

package builder

import (
	"flag"
	"time"

	"github.com/spf13/viper"

	"github.com/uber/jaeger/cmd/collector/app"
)

const (
	collectorQueueSize     = "collector.queue-size"
	collectorNumWorkers    = "collector.num-workers"
	collectorWriteCacheTTL = "collector.write-cache-ttl"
	collectorPort          = "collector.port"
	collectorHTTPPort      = "collector.http-port"
	collectorZipkinHTTPort = "collector.zipkin.http-port"
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
}

// AddFlags adds flags for CollectorOptions
func AddFlags(flags *flag.FlagSet) {
	flags.Int(collectorQueueSize, app.DefaultQueueSize, "The queue size of the collector")
	flags.Int(collectorNumWorkers, app.DefaultNumWorkers, "The number of workers pulling items from the queue")
	flags.Duration(collectorWriteCacheTTL, time.Hour*12, "The duration to wait before rewriting an existing service or operation name")
	flags.Int(collectorPort, 14267, "The tchannel port for the collector service")
	flags.Int(collectorHTTPPort, 14268, "The http port for the collector service")
	flags.Int(collectorZipkinHTTPort, 0, "The http port for the Zipkin collector service e.g. 9411")
}

// InitFromViper initializes CollectorOptions with properties from viper
func (cOpts *CollectorOptions) InitFromViper(v *viper.Viper) *CollectorOptions {
	cOpts.QueueSize = v.GetInt(collectorQueueSize)
	cOpts.NumWorkers = v.GetInt(collectorNumWorkers)
	cOpts.WriteCacheTTL = v.GetDuration(collectorWriteCacheTTL)
	cOpts.CollectorPort = v.GetInt(collectorPort)
	cOpts.CollectorHTTPPort = v.GetInt(collectorHTTPPort)
	cOpts.CollectorZipkinHTTPPort = v.GetInt(collectorZipkinHTTPort)
	return cOpts
}
