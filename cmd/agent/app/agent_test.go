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
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	jmetrics "github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestAgentStartError(t *testing.T) {
	cfg := &Builder{}
	configureSamplingManager(t, cfg, metrics.NullFactory)
	agent, err := cfg.CreateAgent(zap.NewNop(), metrics.NullFactory)
	require.NoError(t, err)
	agent.httpServer.Addr = "bad-address"
	assert.Error(t, agent.Run())
}

func TestAgentSamplingEndpoint(t *testing.T) {
	withRunningAgent(t, func(httpAddr string, errorch chan error) {
		url := fmt.Sprintf("http://%s/sampling?service=abc", httpAddr)
		httpClient := &http.Client{
			Timeout: 100 * time.Millisecond,
		}
		for i := 0; i < 1000; i++ {
			_, err := httpClient.Get(url)
			if err == nil {
				break
			}
			select {
			case err := <-errorch:
				if err != nil {
					t.Fatalf("error from agent: %s", err)
				}
				break
			default:
				time.Sleep(time.Millisecond)
			}
		}
		resp, err := http.Get(url)
		require.NoError(t, err)
		body, err := ioutil.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Equal(t, "tcollector error: no peers available\n", string(body))
	})
}

func TestAgentMetricsEndpoint(t *testing.T) {
	withRunningAgent(t, func(httpAddr string, errorch chan error) {
		url := fmt.Sprintf("http://%s/metrics", httpAddr)
		resp, err := http.Get(url)
		require.NoError(t, err)
		body, err := ioutil.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "# HELP")
	})
}

func withRunningAgent(t *testing.T, testcase func(string, chan error)) {
	resetDefaultPrometheusRegistry()
	cfg := Builder{
		Processors: []ProcessorConfiguration{
			{
				Model:    jaegerModel,
				Protocol: compactProtocol,
				Server: ServerConfiguration{
					HostPort: "127.0.0.1:0",
				},
			},
		},
		HTTPServer: HTTPServerConfiguration{
			HostPort: ":0",
		},
		Metrics: jmetrics.Builder{
			Backend:   "prometheus",
			HTTPRoute: "/metrics",
		},
	}
	logger, logBuf := testutils.NewLogger()
	f, _ := cfg.Metrics.CreateMetricsFactory("jaeger")
	configureSamplingManager(t, &cfg, f)
	agent, err := cfg.CreateAgent(logger, f)
	require.NoError(t, err)
	ch := make(chan error, 2)
	go func() {
		if err := agent.Run(); err != nil {
			t.Errorf("error from agent.Run(): %s", err)
			ch <- err
		}
		close(ch)
	}()

	for i := 0; i < 1000; i++ {
		if agent.HTTPAddr() != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}

	testcase(agent.HTTPAddr(), ch)

	// TODO (wjang) We sleep because the processors in the agent might not have had time to
	// start up yet. If we were to call Stop() before the processors have time to startup,
	// it'll panic because of a DATA RACE between wg.Add() and wg.Wait() in thrift_processor.
	// A real fix for this issue would be to add semaphores to tbuffered_server, thrift_processor,
	// and agent itself. Given all this extra overhead and testing required to get this to work
	// "elegantly", I opted to just sleep here given how unlikely this situation will occur in
	// production.
	time.Sleep(2 * time.Second)

	agent.Stop()
	assert.NoError(t, <-ch)

	for i := 0; i < 1000; i++ {
		if strings.Contains(logBuf.String(), "agent's http server exiting") {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Expecting server exit log")
}

func resetDefaultPrometheusRegistry() {
	// Create and assign a new Prometheus Registerer/Gatherer for each test
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry
}
