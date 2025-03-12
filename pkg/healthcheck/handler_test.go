// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package healthcheck_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
)

func TestStatusString(t *testing.T) {
	tests := map[healthcheck.Status]string{
		healthcheck.Unavailable: "unavailable",
		healthcheck.Ready:       "ready",
		healthcheck.Broken:      "broken",
		healthcheck.Status(-1):  "unknown",
	}
	for k, v := range tests {
		assert.Equal(t, v, k.String())
	}
}

func TestStatusSetGet(t *testing.T) {
	hc := healthcheck.New()
	assert.Equal(t, healthcheck.Unavailable, hc.Get())

	logger, logBuf := testutils.NewLogger()
	hc = healthcheck.New()
	hc.SetLogger(logger)
	assert.Equal(t, healthcheck.Unavailable, hc.Get())

	hc.Ready()
	assert.Equal(t, healthcheck.Ready, hc.Get())
	assert.Equal(t, map[string]string{"level": "info", "msg": "Health Check state change", "status": "ready"}, logBuf.JSONLine(0))
}

func TestHealthCheck_Handler_ContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	healthcheck.New().Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	resp := rec.Result()

	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}
