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

	"github.com/jaegertracing/jaeger/model"
	"github.com/uber/jaeger-lib/metrics"
)

const (
	traceIDByteSize = 16
)

var (
	instance poolSingleton
	once     sync.Once
)

// Wrap the sync.Pool object within a singleton struct.
type poolSingleton struct {
	hashpool *sync.Pool
	bytepool *sync.Pool
}

// downSamplingWriterMetrics is metrics that keeping track of total number of dropped spans
type downSamplingWriterMetrics struct {
	SpansDropped  metrics.Counter `metric:"spans_dropped"`
	SpansAccepted metrics.Counter `metric:"spans_accepted"`
}

// DownSamplingWriter is a span Writer that drops spans with a predefined downSamplingRatio.
type DownSamplingWriter struct {
	spanWriter   Writer
	threshold    uint64
	lengthOfSalt int
	poolInstance poolSingleton
	metrics      *downSamplingWriterMetrics
}

// DownSamplingOptions contains the options for constructing a DownSamplingWriter.
type DownSamplingOptions struct {
	Ratio          float64
	HashSalt       string
	MetricsFactory metrics.Factory
}

// Initialize the singleton if it doesn't exist already.
func getHashPoolInstance(length int) poolSingleton {
	once.Do(func() {
		instance = poolSingleton{
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

// NewDownSamplingWriter creates a DownSamplingWriter.
func NewDownSamplingWriter(spanWriter Writer, downSamplingOptions DownSamplingOptions) *DownSamplingWriter {
	threshold := uint64(downSamplingOptions.Ratio * float64(math.MaxUint64))
	writeMetrics := &downSamplingWriterMetrics{}
	metrics.Init(writeMetrics, downSamplingOptions.MetricsFactory, nil)
	hashSaltBytes := []byte(downSamplingOptions.HashSalt)
	poolInstance := getHashPoolInstance(len(hashSaltBytes) + traceIDByteSize)
	byteSlice := poolInstance.bytepool.Get().(*[]byte)
	copy((*byteSlice)[:len(hashSaltBytes)], hashSaltBytes)
	poolInstance.bytepool.Put(byteSlice)
	return &DownSamplingWriter{
		spanWriter:   spanWriter,
		threshold:    threshold,
		poolInstance: poolInstance,
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
	byteSlice := ds.poolInstance.bytepool.Get().(*[]byte)
	// Currently MarshalTo will only return err if size of traceIDBytes is smaller than 16
	// Since we force traceIDBytes to be size of 16 metrics is not necessary here.
	span.TraceID.MarshalTo((*byteSlice)[ds.lengthOfSalt:])
	hashVal := ds.hashBytes(*byteSlice)
	ds.poolInstance.bytepool.Put(byteSlice)
	return hashVal >= ds.threshold
}

// hashBytes returns the uint64 hash value of byte slice.
func (ds *DownSamplingWriter) hashBytes(bytes []byte) uint64 {
	h := ds.poolInstance.hashpool.Get().(hash.Hash64)
	h.Reset()
	// Currently fnv.Write() doesn't throw any error so metrics is not necessary here.
	h.Write(bytes)
	sum := h.Sum64()
	ds.poolInstance.hashpool.Put(h)
	return sum
}
