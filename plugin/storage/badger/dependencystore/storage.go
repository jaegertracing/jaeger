// Copyright (c) 2018 The Jaeger Authors.
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

package dependencystore

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/gogo/protobuf/proto"
	"github.com/jaegertracing/jaeger/model"
)

const (
	// TODO Maybe these should be visible in the spanstore?
	dependencyKeyPrefix byte = 0xC0 // Dependency PKs have first two bits set to 1
	spanKeyPrefix       byte = 0x80 // All span keys should have first bit set to 1
	sizeOfTraceID            = 16
	encodingTypeBits    byte = 0x0F // UserMeta's last four bits are reserved for encoding type
	jsonEncoding        byte = 0x01 // Last 4 bits of the meta byte are for encoding type
	protoEncoding       byte = 0x02 // Last 4 bits of the meta byte are for encoding type
)

// DependencyStore handles all queries and insertions to Cassandra dependencies
type DependencyStore struct {
	store *badger.DB
}

// NewDependencyStore returns a DependencyStore
func NewDependencyStore(db *badger.DB) *DependencyStore {
	return &DependencyStore{
		store: db,
	}
}

// GetDependencies returns all interservice dependencies, implements DependencyReader
func (s *DependencyStore) GetDependencies(endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	startTs := model.TimeAsEpochMicroseconds(endTs.Add(-1 * lookback))
	beginTs := model.TimeAsEpochMicroseconds(endTs)
	deps := map[string]*model.DependencyLink{}

	// We need to do a full table scan - if this becomes a bottleneck, we can write write an index that describes
	// dependencyKeyPrefix + timestamp + parent + child key and do a key-only seek (which is fast - but requires additional writes)
	err := s.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		it := txn.NewIterator(opts)
		defer it.Close()

		val := []byte{}
		startIndex := []byte{spanKeyPrefix}
		spans := make([]*model.Span, 0)
		prevTraceID := []byte{}
		for it.Seek(startIndex); it.ValidForPrefix(startIndex); it.Next() {
			item := it.Item()

			key := []byte{}
			key = item.KeyCopy(key)

			timestamp := binary.BigEndian.Uint64(key[sizeOfTraceID+1 : sizeOfTraceID+1+8])
			traceID := key[1 : sizeOfTraceID+1]

			if timestamp >= startTs && timestamp <= beginTs {
				val, err := item.ValueCopy(val)
				if err != nil {
					return err
				}

				sp := model.Span{}
				switch item.UserMeta() & encodingTypeBits {
				case jsonEncoding:
					if err := json.Unmarshal(val, &sp); err != nil {
						return err
					}
				case protoEncoding:
					if err := proto.Unmarshal(val, &sp); err != nil {
						return err
					}
				default:
					return fmt.Errorf("Unknown encoding type: %04b", item.UserMeta()&encodingTypeBits)
				}

				if bytes.Equal(prevTraceID, traceID) {
					// Still processing the same one
					spans = append(spans, &sp)
				} else {
					// Process last complete span
					trace := &model.Trace{
						Spans: spans,
					}
					processTrace(deps, trace)

					spans = make([]*model.Span, 0, cap(spans)) // Use previous cap
					spans = append(spans, &sp)
				}
				prevTraceID = traceID
			}
		}
		if len(spans) > 0 {
			trace := &model.Trace{
				Spans: spans,
			}
			processTrace(deps, trace)
		}

		return nil
	})

	return depMapToSlice(deps), err
}

// depMapToSlice modifies the spans to DependencyLink in the same way as the memory storage plugin
func depMapToSlice(deps map[string]*model.DependencyLink) []model.DependencyLink {
	retMe := make([]model.DependencyLink, 0, len(deps))
	for _, dep := range deps {
		retMe = append(retMe, *dep)
	}
	return retMe
}

// processTrace is copy from the memory storage plugin
func processTrace(deps map[string]*model.DependencyLink, trace *model.Trace) {
	for _, s := range trace.Spans {
		parentSpan := seekToSpan(trace, s.ParentSpanID())
		if parentSpan != nil {
			if parentSpan.Process.ServiceName == s.Process.ServiceName {
				continue
			}
			depKey := parentSpan.Process.ServiceName + "&&&" + s.Process.ServiceName
			if _, ok := deps[depKey]; !ok {
				deps[depKey] = &model.DependencyLink{
					Parent:    parentSpan.Process.ServiceName,
					Child:     s.Process.ServiceName,
					CallCount: 1,
				}
			} else {
				deps[depKey].CallCount++
			}
		}
	}
}

func seekToSpan(trace *model.Trace, spanID model.SpanID) *model.Span {
	for _, s := range trace.Spans {
		if s.SpanID == spanID {
			return s
		}
	}
	return nil
}
