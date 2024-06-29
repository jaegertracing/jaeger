// Copyright (c) 2019 The Jaeger Authors.
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

package healthcheck_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/testutils"
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
