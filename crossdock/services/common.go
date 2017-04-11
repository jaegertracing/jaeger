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
	"net/http"
	"time"

	"go.uber.org/zap"

	"code.uber.internal/go-common.git/x/tchannel"

	"golang.org/x/net/context"
)

func healthCheck(logger *zap.Logger, service, healthURL string) {
	for i := 0; i < 100; i++ {
		_, err := http.Get(healthURL)
		if err == nil {
			return
		}
		logger.Warn("Health check failed", zap.String("service", service), zap.Error(err))
		time.Sleep(100 * time.Millisecond)
	}
	logger.Fatal("All health checks failed", zap.String("service", service))
}

func tChannelHealthCheck(logger *zap.Logger, service, hostPort string) {
	cfg := xtchannel.Configuration{DisableHyperbahn: true}
	channel, err := cfg.NewClient("test_driver", nil)
	if err != nil {
		logger.Fatal("Could not initialize tchannel for health check")
	}
	for i := 0; i < 100; i++ {
		err := channel.Ping(context.Background(), hostPort)
		if err == nil {
			return
		}
		logger.Warn("Health check failed", zap.String("service", service), zap.Error(err))
		time.Sleep(100 * time.Millisecond)
	}
	logger.Fatal("All health checks failed", zap.String("service", service))
}

func getTracerServiceName(service string) string {
	return fmt.Sprintf("crossdock-%s", service)
}
