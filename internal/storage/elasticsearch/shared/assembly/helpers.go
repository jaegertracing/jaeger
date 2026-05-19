// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package assembly

import (
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/internal/cache"
)

// LogErrorToSpan records an error on the given OpenTelemetry span.
// If err is nil, this function returns early without recording anything.
func LogErrorToSpan(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// KeyInCache returns true if the key exists in the cache.
func KeyInCache(key string, c cache.Cache) bool {
	return c.Get(key) != nil
}

// WriteCache stores the key in the cache.
func WriteCache(key string, c cache.Cache) {
	c.Put(key, key)
}
