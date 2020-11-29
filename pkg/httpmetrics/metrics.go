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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uber/jaeger-lib/metrics"
)

const concatenation = "$_$"

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

// Wrap returns a handler that wraps the provided one and emits metrics based on the HTTP requests and responses.
// It will record the HTTP response status, HTTP method, duration and path of the call.
// The duration will be reported in metrics.Timer and the rest will be labels on that timer.
func Wrap(h http.Handler, metricsFactory metrics.Factory) http.Handler {
	timers := newRequestDurations(metricsFactory)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		h.ServeHTTP(recorder, r)

		req := recordedRequest{
			status:   strconv.Itoa(recorder.status),
			path:     r.URL.Path,
			method:   r.Method,
			duration: time.Since(start),
		}
		timers.record(req)
	})
}

type recordedRequest struct {
	method   string
	path     string
	status   string
	duration time.Duration
}

type requestDurations struct {
	metrics           metrics.Factory
	timers            map[string]metrics.Timer
	stringBuilderPool *sync.Pool
}

func newRequestDurations(metricsFactory metrics.Factory) *requestDurations {
	return &requestDurations{
		stringBuilderPool: &sync.Pool{
			New: func() interface{} {
				return new(strings.Builder)
			},
		},
		timers:  map[string]metrics.Timer{},
		metrics: metricsFactory,
	}
}

func (r *requestDurations) record(request recordedRequest) {
	cacheKey := r.cacheKey(request)
	timer, ok := r.timers[cacheKey]
	if !ok {
		timer = buildTimer(r.metrics, request)
		r.timers[cacheKey] = timer
	}
	timer.Record(request.duration)
}

func (r *requestDurations) cacheKey(request recordedRequest) string {
	keyBuilder := r.stringBuilderPool.Get().(*strings.Builder)
	defer r.stringBuilderPool.Put(keyBuilder)

	keyBuilder.Reset()
	keyBuilder.WriteString(request.method)
	keyBuilder.WriteString(concatenation)
	keyBuilder.WriteString(request.path)
	keyBuilder.WriteString(concatenation)
	keyBuilder.WriteString(request.status)

	return keyBuilder.String()
}

func buildTimer(metricsFactory metrics.Factory, request recordedRequest) metrics.Timer {
	return metricsFactory.Timer(metrics.TimerOptions{
		Name: "http.request.duration",
		Help: "Duration of HTTP requests",
		Tags: map[string]string{
			"status": request.status,
			"path":   request.path,
			"method": request.method,
		},
	})
}
