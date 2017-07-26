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

	"github.com/uber/jaeger/cmd/collector/app"
)

var (
	// QueueSize is the size of collector's queue
	QueueSize = flag.Int("collector.queue-size", app.DefaultQueueSize, "The queue size of the collector")
	// NumWorkers is the number of internal workers in a collector
	NumWorkers = flag.Int("collector.num-workers", app.DefaultNumWorkers, "The number of workers pulling items from the queue")
	// WriteCacheTTL denotes how often to check and re-write a service or operation name
	WriteCacheTTL = flag.Duration("collector.write-cache-ttl", time.Hour*12, "The duration to wait before rewriting an existing service or operation name")
	// CollectorPort is the port that the collector service listens in on for tchannel requests
	CollectorPort = flag.Int("collector.port", 14267, "The tchannel port for the collector service")
	// CollectorHTTPPort is the port that the collector service listens in on for http requests
	CollectorHTTPPort = flag.Int("collector.http-port", 14268, "The http port for the collector service")
	// CollectorZipkinHTTPPort is the port that the Zipkin collector service listens in on for http requests
	CollectorZipkinHTTPPort = flag.Int("collector.zipkin.http-port", 0, "The http port for the Zipkin collector service e.g. 9411")
)
