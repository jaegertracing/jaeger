// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package sanitizer

import (
	"github.com/uber/jaeger/cmd/collector/app/sanitizer/cache"
	"github.com/uber/jaeger/model"
)

// NewServiceNameSanitizer creates a service name sanitizer.
func NewServiceNameSanitizer(cache cache.Cache) SanitizeSpan {
	sanitizer := serviceNameSanitizer{cache: cache}
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
