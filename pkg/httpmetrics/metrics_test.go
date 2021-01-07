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
	"github.com/uber/jaeger-lib/metrics/metricstest"
)

func TestNewMetricsHandler(t *testing.T) {
	dummyHandlerFunc := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		time.Sleep(time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
		w.WriteHeader(http.StatusTeapot) // any subsequent statuses should be ignored
	})

	mb := metricstest.NewFactory(time.Hour)
	handler := Wrap(dummyHandlerFunc, mb)

	req, err := http.NewRequest(http.MethodGet, "/subdir/qwerty", nil)
	assert.NoError(t, err)
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
