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
