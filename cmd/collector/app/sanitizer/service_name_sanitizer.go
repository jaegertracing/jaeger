// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/cache"
	"github.com/jaegertracing/jaeger/model"
)

// NewServiceNameSanitizer creates a service name sanitizer.
func NewServiceNameSanitizer(c cache.Cache) SanitizeSpan {
	sanitizer := serviceNameSanitizer{cache: c}
	return sanitizer.Sanitize
}

// ServiceNameSanitizer sanitizes the service names in span annotations given a source of truth alias to service cache.
type serviceNameSanitizer struct {
	cache cache.Cache
}

// Sanitize sanitizes the service names in the span annotations.
func (s *serviceNameSanitizer) Sanitize(span *model.Span) *model.Span {
	if s.cache.IsEmpty() {
		return span
	}
	alias := span.Process.ServiceName
	serviceName := s.cache.Get(alias)
	if serviceName != "" {
		span.Process.ServiceName = serviceName
	}
	return span
}
