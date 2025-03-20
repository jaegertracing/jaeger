// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package httpmetrics

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
)

// limit the size of cache for timers to avoid DDOS.
const maxEntries = 1000

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
//
// Do not use with HTTP endpoints that take parameters from URL path, such as `/user/{user_id}`,
// because they will result in high cardinality metrics.
func Wrap(h http.Handler, metricsFactory api.Factory, logger *zap.Logger) http.Handler {
	timers := newRequestDurations(metricsFactory, logger)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		h.ServeHTTP(recorder, r)

		req := recordedRequest{
			key: recordedRequestKey{
				status: strconv.Itoa(recorder.status),
				path:   r.URL.Path,
				method: r.Method,
			},
			duration: time.Since(start),
		}
		timers.record(req)
	})
}

type recordedRequestKey struct {
	method string
	path   string
	status string
}

type recordedRequest struct {
	key      recordedRequestKey
	duration time.Duration
}

type requestDurations struct {
	lock sync.RWMutex

	metrics    api.Factory
	logger     *zap.Logger
	maxEntries int

	timers   map[recordedRequestKey]api.Timer
	fallback api.Timer
}

func newRequestDurations(metricsFactory api.Factory, logger *zap.Logger) *requestDurations {
	r := &requestDurations{
		timers:     make(map[recordedRequestKey]api.Timer),
		metrics:    metricsFactory,
		logger:     logger,
		maxEntries: maxEntries,
	}
	r.fallback = r.getTimer(recordedRequestKey{
		method: "other",
		path:   "other",
		status: "other",
	})
	return r
}

func (r *requestDurations) record(request recordedRequest) {
	timer := r.getTimer(request.key)
	timer.Record(request.duration)
}

func (r *requestDurations) getTimer(cacheKey recordedRequestKey) api.Timer {
	r.lock.RLock()
	timer, ok := r.timers[cacheKey]
	size := len(r.timers)
	r.lock.RUnlock()
	if !ok {
		if size >= r.maxEntries {
			return r.fallback
		}
		r.lock.Lock()
		timer, ok = r.timers[cacheKey]
		if !ok {
			timer = r.buildTimer(r.metrics, cacheKey)
			r.timers[cacheKey] = timer
		}
		r.lock.Unlock()
	}
	return timer
}

func (r *requestDurations) buildTimer(metricsFactory api.Factory, key recordedRequestKey) (out api.Timer) {
	// deal with https://github.com/jaegertracing/jaeger/issues/2944
	defer func() {
		if err := recover(); err != nil {
			r.logger.Error("panic in metrics factory trying to create a timer", zap.Any("error", err))
			out = api.NullTimer
		}
	}()

	out = metricsFactory.Timer(api.TimerOptions{
		Name: "http.request.duration",
		Help: "Duration of HTTP requests",
		Tags: map[string]string{
			"status": key.status,
			"path":   key.path,
			"method": key.method,
		},
	})
	return out
}
