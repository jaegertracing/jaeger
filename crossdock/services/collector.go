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

package services

import (
	"fmt"
	"os/exec"

	"go.uber.org/zap"
)

const (
	collectorService = "Collector"

	collectorCmd      = "/go/cmd/jaeger-collector %s &"
	collectorHostPort = "localhost:14267"
)

// InitializeCollector initializes the jaeger collector
func InitializeCollector(logger *zap.Logger) {
	cmd := exec.Command("/bin/bash", "-c",
		fmt.Sprintf(collectorCmd, "-cassandra.keyspace=jaeger -cassandra.servers=cassandra -cassandra.connections-per-host=1"))
	if err := cmd.Run(); err != nil {
		logger.Fatal("Failed to initialize collector service", zap.Error(err))
	}
	tChannelHealthCheck(logger, collectorService, collectorHostPort)
}
