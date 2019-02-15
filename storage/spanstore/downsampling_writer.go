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

package spanstore

import (
	"hash"
	"hash/fnv"
	"math"

	"github.com/jaegertracing/jaeger/model"
)

// DownSamplingWriter is a span Writer that tries to drop spans with
// predefined downSamplingRatio
type DownSamplingWriter struct {
	spanWriter Writer
	threshold  uint64
	fnvHash    hash.Hash64
}

// NewDownSamplingWriter creates a DownSamplingWriter
func NewDownSamplingWriter(spanWriter Writer, downSamplingRatio float64) *DownSamplingWriter {
	threshold := uint64(downSamplingRatio * float64(math.MaxUint64))
	// fnv hash computes uint64 as hash value
	fnvHash := fnv.New64a()
	return &DownSamplingWriter{
		spanWriter: spanWriter,
		threshold:  threshold,
		fnvHash:    fnvHash,
	}
}

// WriteSpan calls WriteSpan on wrapped span writer.
func (c *DownSamplingWriter) WriteSpan(span *model.Span) error {
	if c.threshold == 0 {
		return c.spanWriter.WriteSpan(span)
	}
	hashVal := HashBytes(c.fnvHash, []byte(span.TraceID.String()))
	if hashVal < c.threshold {
		// Downsampling writer prevents writing span when hashVal falls
		// in the range of (0, threshold)
		return nil
	}
	return c.spanWriter.WriteSpan(span)
}
