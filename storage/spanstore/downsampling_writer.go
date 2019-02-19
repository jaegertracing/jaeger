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

// DownSamplingWriter is a span Writer that drops spans with a predefined downSamplingRatio.
type DownSamplingWriter struct {
	spanWriter Writer
	threshold  uint64
	hashSalt   string
	instance   *hashPoolSingleton
	metrics    *downsamplingWriterMetrics
}

// downsamplingWriterMetrics keeps track of:
// errors converting trace ID into a binary representation,
// errors hashing bytes to uint64.
type downsamplingWriterMetrics struct {
	MarshalingErrors metrics.Counter `metric:"MarshalTo_errors"`
	HashingErrors    metrics.Counter `metric:"hashing_errors"`
}

// Wrap the sync.Pool object within a singleton struct.
type hashPoolSingleton struct {
	pool *sync.Pool
}

var instance *hashPoolSingleton
var once sync.Once

// Initialize the singleton if it doesn't exist already.
func getHashPoolInstance() *hashPoolSingleton {
	once.Do(func() {
		instance = &hashPoolSingleton{
			pool: &sync.Pool{
				New: func() interface{} {
					return fnv.New64a()
				},
			},
		}
	})
	return instance
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
	downsamplingMetrics := &downsamplingWriterMetrics{}
	err := metrics.Init(downsamplingMetrics, downSamplingOptions.MetricsFactory, nil)
	if err != nil {
		return nil
	}
	return &DownSamplingWriter{
		spanWriter: spanWriter,
		threshold:  threshold,
		hashSalt:   downSamplingOptions.HashSalt,
		instance:   getHashPoolInstance(),
		metrics:    downsamplingMetrics,
	}
}

// WriteSpan calls WriteSpan on wrapped span writer.
func (ds *DownSamplingWriter) WriteSpan(span *model.Span) error {
	// No downSampling when threshold equals maxuint64
	if ds.threshold == math.MaxUint64 {
		return ds.spanWriter.WriteSpan(span)
	}

	hashSaltBytes := []byte(ds.hashSalt)
	// traceID marshal 16 bytes
	traceIDBytes := make([]byte, 16)
	length, err := span.TraceID.MarshalTo(traceIDBytes)
	if err != nil || length != 16 {
		// No Downsamping when there's error marshaling.
		ds.metrics.MarshalingErrors.Inc(1)
		return ds.spanWriter.WriteSpan(span)
	}
	hashVal := ds.hashBytes(append(hashSaltBytes, traceIDBytes...))

	if hashVal >= ds.threshold {
		// Drops spans when hashVal falls beyond computed threshold.
		return nil
	}
	return ds.spanWriter.WriteSpan(span)
}

//hashBytes returns the uint64 hash value of bytes slice.
func (ds *DownSamplingWriter) hashBytes(bytes []byte) uint64 {
	h := ds.instance.pool.Get().(hash.Hash64)
	defer ds.instance.pool.Put(h)
	h.Reset()
	_, err := h.Write(bytes)
	if err != nil {
		// No downsampling when there's error hashing.
		ds.metrics.HashingErrors.Inc(1)
		return math.MaxUint64
	}
	return h.Sum64()
}
