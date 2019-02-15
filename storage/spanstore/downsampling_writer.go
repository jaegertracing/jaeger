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
)

// DownSamplingWriter is a span Writer that drops spans with a predefined downSamplingRatio.
type DownSamplingWriter struct {
	spanWriter Writer
	threshold  uint64
	hashSalt   string
	fnvHash    hash.Hash64
	lock       sync.Mutex
}

// DownSamplingOptions contains the options for constructing a DownSamplingWriter.
type DownSamplingOptions struct {
	Ratio    float64
	HashSalt string
}

// NewDownSamplingWriter creates a DownSamplingWriter.
func NewDownSamplingWriter(spanWriter Writer, downSamplingOptions DownSamplingOptions) *DownSamplingWriter {
	threshold := uint64(downSamplingOptions.Ratio * float64(math.MaxUint64))
	fnvHash := fnv.New64a()
	return &DownSamplingWriter{
		spanWriter: spanWriter,
		threshold:  threshold,
		hashSalt:   downSamplingOptions.HashSalt,
		fnvHash:    fnvHash,
	}
}

// WriteSpan calls WriteSpan on wrapped span writer.
func (ds *DownSamplingWriter) WriteSpan(span *model.Span) error {
	if ds.threshold == math.MaxUint64 {
		return ds.spanWriter.WriteSpan(span)
	}

	hashSaltBytes := []byte(ds.hashSalt)
	// traceID marshal 16 bytes
	traceIDBytes := make([]byte, 16)
	length, err := span.TraceID.MarshalTo(traceIDBytes)
	if err != nil || length != 16 {
		// No Downsamping when there's error marshaling.
		return ds.spanWriter.WriteSpan(span)
	}
	hashVal := ds.hashBytes(append(hashSaltBytes, traceIDBytes...))

	if hashVal >= ds.threshold {
		// Drops spans when hashVal falls beyond computed threshold.
		return nil
	}
	return ds.spanWriter.WriteSpan(span)
}

func (ds *DownSamplingWriter) hashBytes(bytes []byte) uint64 {
	ds.lock.Lock()
	defer ds.lock.Unlock()
	ds.fnvHash.Reset()
	_, err := ds.fnvHash.Write(bytes)
	if err != nil {
		return math.MaxUint64
	}
	return ds.fnvHash.Sum64()
}
