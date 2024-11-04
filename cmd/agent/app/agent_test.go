// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics/metricsbuilder"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestAgentStartError(t *testing.T) {
	cfg := &Builder{}
	agent, err := cfg.CreateAgent(fakeCollectorProxy{}, zap.NewNop(), metrics.NullFactory)
	require.NoError(t, err)
	agent.httpServer.Addr = "bad-address"
	require.Error(t, agent.Run())
}

func TestAgentSamplingEndpoint(t *testing.T) {
	withRunningAgent(t, func(httpAddr string, errorch chan error) {
		url := fmt.Sprintf("http://%s/sampling?service=abc", httpAddr)
		httpClient := &http.Client{
			Timeout: 100 * time.Millisecond,
		}
	wait_loop:
		for i := 0; i < 1000; i++ {
			_, err := httpClient.Get(url)
			if err == nil {
				break
			}
			select {
			case err := <-errorch:
				require.NoError(t, err, "error from agent")
				break wait_loop
			default:
				time.Sleep(time.Millisecond)
			}
		}
		resp, err := http.Get(url)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "collector error: no peers available\n", string(body))
	})
}

func TestAgentMetricsEndpoint(t *testing.T) {
	withRunningAgent(t, func(httpAddr string, _ chan error) {
		url := fmt.Sprintf("http://%s/metrics", httpAddr)
		resp, err := http.Get(url)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
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
			HostPort: "127.0.0.1:0",
		},
	}

	logger, logBuf := testutils.NewLogger()
	mBldr := &metricsbuilder.Builder{HTTPRoute: "/metrics", Backend: "prometheus"}
	metricsFactory, err := mBldr.CreateMetricsFactory("jaeger")
	require.NoError(t, err)
	agent, err := cfg.CreateAgent(fakeCollectorProxy{}, logger, metricsFactory)
	require.NoError(t, err)
	if h := mBldr.Handler(); metricsFactory != nil && h != nil {
		logger.Info("Registering metrics handler with HTTP server", zap.String("route", mBldr.HTTPRoute))
		agent.GetHTTPRouter().Handle(mBldr.HTTPRoute, h).Methods(http.MethodGet)
	}
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

	agent.Stop()
	require.NoError(t, <-ch)

	for i := 0; i < 1000; i++ {
		if strings.Contains(logBuf.String(), "agent's http server exiting") {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Expecting server exit log")
}

func TestStartStopRace(t *testing.T) {
	resetDefaultPrometheusRegistry()
	cfg := Builder{
		Processors: []ProcessorConfiguration{
			{
				Model:    jaegerModel,
				Protocol: compactProtocol,
				Workers:  1,
				Server: ServerConfiguration{
					HostPort: "127.0.0.1:0",
				},
			},
		},
	}
	logger, logBuf := testutils.NewEchoLogger(t)
	mBldr := &metricsbuilder.Builder{HTTPRoute: "/metrics", Backend: "prometheus"}
	metricsFactory, err := mBldr.CreateMetricsFactory("jaeger")
	require.NoError(t, err)
	agent, err := cfg.CreateAgent(fakeCollectorProxy{}, logger, metricsFactory)
	require.NoError(t, err)

	// This test attempts to hit the data race bug when Stop() is called
	// immediately after Run(). We had a bug like that which is now fixed:
	// https://github.com/jaegertracing/jaeger/issues/1624
	// Before the bug was fixed this test was failing as expected when
	// run with -race flag.

	require.NoError(t, agent.Run())

	t.Log("stopping agent")
	agent.Stop()

	for i := 0; i < 1000; i++ {
		if strings.Contains(logBuf.String(), "agent's http server exiting") {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("Expecting server exit log")
}

func TestStartStopWithSocketBufferSet(t *testing.T) {
	resetDefaultPrometheusRegistry()
	cfg := Builder{
		Processors: []ProcessorConfiguration{
			{
				Model:    jaegerModel,
				Protocol: compactProtocol,
				Workers:  1,
				Server: ServerConfiguration{
					HostPort:         "127.0.0.1:0",
					SocketBufferSize: 10,
				},
			},
		},
	}
	mBldr := &metricsbuilder.Builder{HTTPRoute: "/metrics", Backend: "prometheus"}
	metricsFactory, err := mBldr.CreateMetricsFactory("jaeger")
	require.NoError(t, err)
	agent, err := cfg.CreateAgent(fakeCollectorProxy{}, zap.NewNop(), metricsFactory)
	require.NoError(t, err)

	require.NoError(t, agent.Run())

	t.Log("stopping agent")
	agent.Stop()
}

func resetDefaultPrometheusRegistry() {
	// Create and assign a new Prometheus Registerer/Gatherer for each test
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry
}
