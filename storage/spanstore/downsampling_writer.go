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
	"context"
	"hash"
	"hash/fnv"
	"math"
	"math/big"
	"sync"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/model"
)

const defaultHashSalt = "downsampling-default-salt"

var (
	traceIDByteSize = (&model.TraceID{}).Size()
)

// hasher includes data we want to put in sync.Pool.
type hasher struct {
	hash   hash.Hash64
	buffer []byte
}

// downsamplingWriterMetrics keeping track of total number of dropped spans and accepted spans.
type downsamplingWriterMetrics struct {
	SpansDropped  metrics.Counter `metric:"spans_dropped"`
	SpansAccepted metrics.Counter `metric:"spans_accepted"`
}

// DownsamplingWriter is a span Writer that drops spans with a predefined downsamplingRatio.
type DownsamplingWriter struct {
	spanWriter Writer
	metrics    downsamplingWriterMetrics
	sampler    *Sampler
}

// DownsamplingOptions contains the options for constructing a DownsamplingWriter.
type DownsamplingOptions struct {
	Ratio          float64
	HashSalt       string
	MetricsFactory metrics.Factory
}

// NewDownsamplingWriter creates a DownsamplingWriter.
func NewDownsamplingWriter(spanWriter Writer, downsamplingOptions DownsamplingOptions) *DownsamplingWriter {
	writeMetrics := &downsamplingWriterMetrics{}
	metrics.Init(writeMetrics, downsamplingOptions.MetricsFactory, nil)
	return &DownsamplingWriter{
		sampler:    NewSampler(downsamplingOptions.Ratio, downsamplingOptions.HashSalt),
		spanWriter: spanWriter,
		metrics:    *writeMetrics,
	}
}

// WriteSpan calls WriteSpan on wrapped span writer.
func (ds *DownsamplingWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	if !ds.sampler.ShouldSample(span) {
		// Drops spans when hashVal falls beyond computed threshold.
		ds.metrics.SpansDropped.Inc(1)
		return nil
	}
	ds.metrics.SpansAccepted.Inc(1)
	return ds.spanWriter.WriteSpan(ctx, span)
}

// hashBytes returns the uint64 hash value of byte slice.
func (h *hasher) hashBytes() uint64 {
	h.hash.Reset()
	// Currently fnv.Write() implementation doesn't throw any error so metric is not necessary here.
	_, _ = h.hash.Write(h.buffer)
	return h.hash.Sum64()
}

// Sampler decides if we should sample a span
type Sampler struct {
	hasherPool   *sync.Pool
	lengthOfSalt int
	threshold    uint64
}

// NewSampler creates SamplingExecutor
func NewSampler(ratio float64, hashSalt string) *Sampler {
	if hashSalt == "" {
		hashSalt = defaultHashSalt
	}
	hashSaltBytes := []byte(hashSalt)
	pool := &sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, len(hashSaltBytes)+traceIDByteSize)
			copy(buffer, hashSaltBytes)
			return &hasher{
				hash:   fnv.New64a(),
				buffer: buffer,
			}
		},
	}
	return &Sampler{
		threshold:    calculateThreshold(ratio),
		hasherPool:   pool,
		lengthOfSalt: len(hashSaltBytes),
	}
}

func calculateThreshold(ratio float64) uint64 {
	// Use big.Float and big.Int to calculate threshold because directly convert
	// math.MaxUint64 to float64 will cause digits/bits to be cut off if the converted value
	// doesn't fit into bits that are used to store digits for float64 in Golang
	boundary := new(big.Float).SetInt(new(big.Int).SetUint64(math.MaxUint64))
	res, _ := boundary.Mul(boundary, big.NewFloat(ratio)).Uint64()
	return res
}

// ShouldSample decides if a span should be sampled
func (s *Sampler) ShouldSample(span *model.Span) bool {
	hasherInstance := s.hasherPool.Get().(*hasher)
	// Currently MarshalTo will only return err if size of traceIDBytes is smaller than 16
	// Since we force traceIDBytes to be size of 16 metrics is not necessary here.
	_, _ = span.TraceID.MarshalTo(hasherInstance.buffer[s.lengthOfSalt:])
	hashVal := hasherInstance.hashBytes()
	s.hasherPool.Put(hasherInstance)
	return hashVal <= s.threshold
}
