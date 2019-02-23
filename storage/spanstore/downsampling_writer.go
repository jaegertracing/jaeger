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

// DownSamplingWriter is a span Writer that drops spans with a predefined downSamplingRatio.
type DownSamplingWriter struct {
	spanWriter   Writer
	threshold    uint64
	hashSalt     []byte
	poolInstance *poolSingleton
	metrics      *downSamplingWriterMetrics
}

// downSamplingWriterMetrics is metrics that keeping track of total number of dropped spans
type downSamplingWriterMetrics struct {
	SpansDropped metrics.Counter `metric:"spans_dropped"`
}

// Wrap the sync.Pool object within a singleton struct.
type poolSingleton struct {
	hashpool *sync.Pool
	bytepool *sync.Pool
}

var instance *poolSingleton
var once sync.Once

// Initialize the singleton if it doesn't exist already.
func getHashPoolInstance(length int) *poolSingleton {
	once.Do(func() {
		instance = &poolSingleton{
			hashpool: &sync.Pool{
				New: func() interface{} {
					return fnv.New64a()
				},
			},
			bytepool: &sync.Pool{
				New: func() interface{} {
					byteArray := make([]byte, length)
					return &byteArray
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
	var threshold uint64 = math.MaxUint64
	if downSamplingOptions.Ratio < 1.0 {
		threshold = uint64(downSamplingOptions.Ratio * float64(math.MaxUint64))
	}
	writeMetrics := &downSamplingWriterMetrics{}
	metrics.Init(writeMetrics, downSamplingOptions.MetricsFactory, nil)
	hashSaltBytes := []byte(downSamplingOptions.HashSalt)
	return &DownSamplingWriter{
		spanWriter:   spanWriter,
		threshold:    threshold,
		hashSalt:     hashSaltBytes,
		poolInstance: getHashPoolInstance(len(hashSaltBytes) + traceIDByteSize),
		metrics:      writeMetrics,
	}
}

// WriteSpan calls WriteSpan on wrapped span writer.
func (ds *DownSamplingWriter) WriteSpan(span *model.Span) error {
	// No downSampling when threshold equals maxuint64
	if ds.threshold == math.MaxUint64 {
		return ds.spanWriter.WriteSpan(span)
	}

	byteSlice := ds.poolInstance.bytepool.Get().(*[]byte)
	copy((*byteSlice)[:len(ds.hashSalt)], ds.hashSalt)

	// Currently MarshalTo will only return err if size of traceIDBytes is smaller than 16
	// Since we force traceIDBytes to be size of 16 metrics is not necessary here.
	span.TraceID.MarshalTo((*byteSlice)[len(ds.hashSalt):])
	hashVal := ds.hashBytes(*byteSlice)
	ds.poolInstance.bytepool.Put(byteSlice)
	if hashVal >= ds.threshold {
		// Drops spans when hashVal falls beyond computed threshold.
		ds.metrics.SpansDropped.Inc(1)
		return nil
	}
	return ds.spanWriter.WriteSpan(span)
}

//hashBytes returns the uint64 hash value of bytes slice.
func (ds *DownSamplingWriter) hashBytes(bytes []byte) uint64 {
	h := ds.poolInstance.hashpool.Get().(hash.Hash64)
	h.Reset()
	// Currently fnv.Write() doesn't throw any error so metrics is not necessary here.
	h.Write(bytes)
	sum := h.Sum64()
	ds.poolInstance.hashpool.Put(h)
	return sum
}
