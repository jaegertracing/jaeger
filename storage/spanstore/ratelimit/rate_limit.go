// Copyright (c) 2018 Uber Technologies, Inc.
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

package ratelimit

import (
	"errors"

	rlImpl "go.uber.org/ratelimit"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var errInvalidRate = errors.New("rate must be a positive integer")

type rateLimitedWriter struct {
	writer  spanstore.Writer
	limiter rlImpl.Limiter
}

// NewRateLimitedWriter decorates a Writer with a rate limiter in order to limit
// the number of writes per second.
func NewRateLimitedWriter(writer spanstore.Writer, rate int) (spanstore.Writer, error) {
	if rate <= 0 {
		return nil, errInvalidRate
	}

	return &rateLimitedWriter{
		writer:  writer,
		limiter: rlImpl.New(rate),
	}, nil
}

// WriteSpan wraps the write span method of the inner writer, but invokes the
// rate limiter before each write.
func (r *rateLimitedWriter) WriteSpan(s *model.Span) error {
	r.limiter.Take()
	return r.writer.WriteSpan(s)
}
