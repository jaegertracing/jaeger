// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func TestZapLoggerDoesNotCaptureBodies(t *testing.T) {
	// The logger records the request line but never the bodies, so the pool must
	// not buffer and duplicate them — at any level, including debug.
	for _, level := range []string{"debug", "info", "error"} {
		t.Run(level, func(t *testing.T) {
			l := newZapLogger(level, zap.NewNop())
			assert.False(t, l.RequestBodyEnabled())
			assert.False(t, l.ResponseBodyEnabled())
		})
	}
}

func TestZapLoggerLogRoundTrip(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://es:9200/_cluster/health", http.NoBody)
	require.NoError(t, err)

	t.Run("success logs at info", func(t *testing.T) {
		core, logs := observer.New(zap.InfoLevel)
		l := newZapLogger("info", zap.New(core))
		res := &http.Response{StatusCode: http.StatusOK}
		require.NoError(t, l.LogRoundTrip(req, res, nil, time.Now(), 3*time.Millisecond))

		entries := logs.All()
		require.Len(t, entries, 1)
		assert.Equal(t, zap.InfoLevel, entries[0].Level)
		assert.Equal(t, "Elasticsearch request", entries[0].Message)
		assert.Equal(t, int64(http.StatusOK), entries[0].ContextMap()["status_code"])
	})

	t.Run("failure logs at error with the error", func(t *testing.T) {
		core, logs := observer.New(zap.InfoLevel)
		l := newZapLogger("info", zap.New(core))
		require.NoError(t, l.LogRoundTrip(req, nil, errors.New("boom"), time.Now(), time.Millisecond))

		entries := logs.All()
		require.Len(t, entries, 1)
		assert.Equal(t, zap.ErrorLevel, entries[0].Level)
		assert.Equal(t, "boom", entries[0].ContextMap()["error"])
	})

	t.Run("error level mutes successful round trips", func(t *testing.T) {
		core, logs := observer.New(zap.InfoLevel)
		// newZapLogger raises the threshold to error, so an info-level success is dropped.
		l := newZapLogger("error", zap.New(core))
		res := &http.Response{StatusCode: http.StatusOK}
		require.NoError(t, l.LogRoundTrip(req, res, nil, time.Now(), time.Millisecond))
		assert.Empty(t, logs.All(), "log_level=error must not log successful round trips")

		require.NoError(t, l.LogRoundTrip(req, res, errors.New("boom"), time.Now(), time.Millisecond))
		assert.Len(t, logs.All(), 1, "log_level=error must still log failures")
	})
}

// TestNewClientAttachesRequestLogger drives a real client so the pool's logger
// wiring is exercised end to end: with log_level=info, every request is logged.
func TestNewClientAttachesRequestLogger(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	core, logs := observer.New(zap.InfoLevel)
	c, err := NewClient(t.Context(), &config.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "info",
		Version:  uint(es.ElasticV7), // pin so NewClient doesn't probe
	}, zap.New(core), nil)
	require.NoError(t, err)

	_, err = c.request(t.Context(), elasticRequest{method: http.MethodGet, endpoint: "_cluster/health"})
	require.NoError(t, err)

	assert.NotEmpty(t, logs.FilterMessage("Elasticsearch request").All(), "log_level=info must log the request through zap")
}

// TestNewClientNoRequestLoggerWhenLevelEmpty pins that an empty log_level attaches
// no logger, so nothing is logged.
func TestNewClientNoRequestLoggerWhenLevelEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	core, logs := observer.New(zap.DebugLevel)
	c, err := NewClient(t.Context(), &config.Configuration{
		Servers: []string{server.URL},
		Version: uint(es.ElasticV7),
	}, zap.New(core), nil)
	require.NoError(t, err)

	_, err = c.request(t.Context(), elasticRequest{method: http.MethodGet, endpoint: "_cluster/health"})
	require.NoError(t, err)

	assert.Empty(t, logs.FilterMessage("Elasticsearch request").All())
}
