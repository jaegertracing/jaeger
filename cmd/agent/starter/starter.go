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

package starter

import (
	"go.uber.org/zap"

	"fmt"
	"github.com/uber/jaeger-lib/metrics"
	agentApp "github.com/uber/jaeger/cmd/agent/app"
	collector "github.com/uber/jaeger/cmd/collector/app/builder"
)

// StartAgent starts a new agent daemon to run on the host, based on the given parameters.
func StartAgent(logger *zap.Logger, baseFactory metrics.Factory, builder *agentApp.Builder) {
	metricsFactory := baseFactory.Namespace("jaeger-agent", nil)

	if builder.CollectorHostPort == "" {
		builder.CollectorHostPort = fmt.Sprintf("127.0.0.1:%d", *collector.CollectorPort)
	}
	agent, err := builder.CreateAgent(metricsFactory, logger)
	if err != nil {
		logger.Fatal("Unable to initialize Jaeger Agent", zap.Error(err))
	}

	logger.Info("Starting agent")
	if err := agent.Run(); err != nil {
		logger.Fatal("Failed to run the agent", zap.Error(err))
	}
}
