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

package main

import (
	"flag"
	"runtime"

	"github.com/uber/jaeger-lib/metrics/go-kit"
	"github.com/uber/jaeger-lib/metrics/go-kit/expvar"
	agentApp "github.com/uber/jaeger/cmd/agent/app"
	agentStarter "github.com/uber/jaeger/cmd/agent/starter"
	"go.uber.org/zap"
)

const (
	serviceName = "jaeger-agent"
)

// agent/main starts the agent on the host
func main() {
	logger, _ := zap.NewProduction()
	metricsFactory := xkit.Wrap(serviceName, expvar.NewFactory(10))

	builder := agentApp.NewBuilder()
	builder.Bind(flag.CommandLine)
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	agentStarter.StartAgent(logger, metricsFactory, builder)

	select {}
}
