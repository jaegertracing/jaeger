// Copyright (c) 2019 The Jaeger Authors.
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
	"sync"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/model"
)

const (
	traceIDByteSize = 16
)

// hasher includes data we want to put in sync.Pool
type hasher struct {
	fnv       hash.Hash64
	byteArray []byte
}

// downsamplingWriterMetrics keeping track of total number of dropped spans and accepted spans
type downsamplingWriterMetrics struct {
	SpansDropped  metrics.Counter `metric:"spans_dropped"`
	SpansAccepted metrics.Counter `metric:"spans_accepted"`
}

// DownSamplingWriter is a span Writer that drops spans with a predefined downSamplingRatio.
type DownSamplingWriter struct {
	spanWriter   Writer
	threshold    uint64
	lengthOfSalt int
	pool         *sync.Pool
	metrics      *downsamplingWriterMetrics
}

// DownSamplingOptions contains the options for constructing a DownSamplingWriter.
type DownSamplingOptions struct {
	Ratio          float64
	HashSalt       string
	MetricsFactory metrics.Factory
}

// NewDownSamplingWriter creates a DownSamplingWriter.
func NewDownSamplingWriter(spanWriter Writer, downSamplingOptions DownSamplingOptions) *DownSamplingWriter {
	threshold := uint64(downSamplingOptions.Ratio * float64(math.MaxUint64))
	writeMetrics := &downsamplingWriterMetrics{}
	metrics.Init(writeMetrics, downSamplingOptions.MetricsFactory, nil)
	hashSaltBytes := []byte(downSamplingOptions.HashSalt)
	pool := &sync.Pool{
		New: func() interface{} {
			byteArray := make([]byte, len(hashSaltBytes)+traceIDByteSize)
			copy(byteArray[:len(hashSaltBytes)], hashSaltBytes)
			return &hasher{
				fnv:       fnv.New64a(),
				byteArray: byteArray,
			}
		},
	}

	return &DownSamplingWriter{
		spanWriter:   spanWriter,
		threshold:    threshold,
		pool:         pool,
		metrics:      writeMetrics,
		lengthOfSalt: len(hashSaltBytes),
	}
}

// WriteSpan calls WriteSpan on wrapped span writer.
func (ds *DownSamplingWriter) WriteSpan(span *model.Span) error {
	if ds.shouldDownsample(span) {
		// Drops spans when hashVal falls beyond computed threshold.
		ds.metrics.SpansDropped.Inc(1)
		return nil
	}
	ds.metrics.SpansAccepted.Inc(1)
	return ds.spanWriter.WriteSpan(span)
}

func (ds *DownSamplingWriter) shouldDownsample(span *model.Span) bool {
	hasherInstance := ds.pool.Get().(*hasher)
	// Currently MarshalTo will only return err if size of traceIDBytes is smaller than 16
	// Since we force traceIDBytes to be size of 16 metrics is not necessary here.
	byteSlice := hasherInstance.byteArray
	// Currently we are always passing byte slice with size of 16 which won't trigger the underlining error
	_, _ = span.TraceID.MarshalTo((byteSlice)[ds.lengthOfSalt:])
	hashVal := hasherInstance.hashBytes(byteSlice)
	ds.pool.Put(hasherInstance)
	return hashVal >= ds.threshold
}

// hashBytes returns the uint64 hash value of byte slice.
func (h *hasher) hashBytes(bytes []byte) uint64 {
	h.fnv.Reset()
	// Currently fnv.Write() doesn't throw any error so metrics is not necessary here.
	_, _ = h.fnv.Write(bytes)
	sum := h.fnv.Sum64()
	return sum
}
