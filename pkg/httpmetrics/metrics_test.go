// Copyright (c) 2020 The Jaeger Authors.
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

package httpmetrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics/prometheus"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestNewMetricsHandler(t *testing.T) {
	dummyHandlerFunc := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		time.Sleep(time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
		w.WriteHeader(http.StatusTeapot) // any subsequent statuses should be ignored
	})

	mb := metricstest.NewFactory(time.Hour)
	defer mb.Stop()
	handler := Wrap(dummyHandlerFunc, mb, zap.NewNop())

	req, err := http.NewRequest(http.MethodGet, "/subdir/qwerty", nil)
	require.NoError(t, err)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	for i := 0; i < 1000; i++ {
		_, gauges := mb.Snapshot()
		if _, ok := gauges["http.request.duration|method=GET|path=/subdir/qwerty|status=202.P999"]; ok {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}

	assert.Fail(t, "gauge hasn't been updated within a reasonable amount of time")
}

func TestMaxEntries(t *testing.T) {
	mf := metricstest.NewFactory(time.Hour)
	defer mf.Stop()
	r := newRequestDurations(mf, zap.NewNop())
	r.maxEntries = 1
	r.record(recordedRequest{
		key: recordedRequestKey{
			path: "/foo",
		},
		duration: time.Millisecond,
	})
	r.lock.RLock()
	size := len(r.timers)
	r.lock.RUnlock()
	assert.Equal(t, 1, size)
}

func TestIllegalPrometheusLabel(t *testing.T) {
	dummyHandlerFunc := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		time.Sleep(time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
		w.WriteHeader(http.StatusTeapot) // any subsequent statuses should be ignored
	})

	mf := prometheus.New().Namespace(metrics.NSOptions{})
	handler := Wrap(dummyHandlerFunc, mf, zap.NewNop())

	invalidUtf8 := []byte{0xC0, 0xAE, 0xC0, 0xAE}
	req, err := http.NewRequest(http.MethodGet, string(invalidUtf8), nil)
	require.NoError(t, err)
	handler.ServeHTTP(httptest.NewRecorder(), req)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
