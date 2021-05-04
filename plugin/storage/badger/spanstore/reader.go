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

package spanstore

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/dgraph-io/badger"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

// Most of these errors are common with the ES and Cassandra backends. Each backend has slightly different validation rules.

var (
	// ErrServiceNameNotSet occurs when attempting to query with an empty service name
	ErrServiceNameNotSet = errors.New("service name must be set")

	// ErrStartTimeMinGreaterThanMax occurs when start time min is above start time max
	ErrStartTimeMinGreaterThanMax = errors.New("min start time is above max")

	// ErrDurationMinGreaterThanMax occurs when duration min is above duration max
	ErrDurationMinGreaterThanMax = errors.New("min duration is above max")

	// ErrMalformedRequestObject occurs when a request object is nil
	ErrMalformedRequestObject = errors.New("malformed request object")

	// ErrStartAndEndTimeNotSet occurs when start time and end time are not set
	ErrStartAndEndTimeNotSet = errors.New("start and end time must be set")

	// ErrUnableToFindTraceIDAggregation occurs when an aggregation query for TraceIDs fail.
	ErrUnableToFindTraceIDAggregation = errors.New("could not find aggregation of traceIDs")

	// ErrNotSupported during development, don't support every option - yet
	ErrNotSupported = errors.New("this query parameter is not supported yet")

	// ErrInternalConsistencyError indicates internal data consistency issue
	ErrInternalConsistencyError = errors.New("internal data consistency issue")
)

const (
	defaultNumTraces = 100
	sizeOfTraceID    = 16
	encodingTypeBits = 0x0F
)

// TraceReader reads traces from the local badger store
type TraceReader struct {
	store *badger.DB
	cache *CacheStore
}

// executionPlan is internal structure to track the index filtering
type executionPlan struct {
	startTimeMin []byte
	startTimeMax []byte

	limit int

	// mergeOuter is the result of merge-join of inner and outer result sets
	mergeOuter [][]byte

	// hashOuter is the hashmap for hash-join of outer resultset
	hashOuter map[model.TraceID]struct{}
}

// NewTraceReader returns a TraceReader with cache
func NewTraceReader(db *badger.DB, c *CacheStore) *TraceReader {
	return &TraceReader{
		store: db,
		cache: c,
	}
}

func decodeValue(val []byte, encodeType byte) (*model.Span, error) {
	sp := model.Span{}
	switch encodeType {
	case jsonEncoding:
		if err := json.Unmarshal(val, &sp); err != nil {
			return nil, err
		}
	case protoEncoding:
		if err := sp.Unmarshal(val); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown encoding type: %#02x", encodeType)
	}
	return &sp, nil
}

// getTraces enriches TraceIDs to Traces
func (r *TraceReader) getTraces(traceIDs []model.TraceID) ([]*model.Trace, error) {
	// Get by PK
	traces := make([]*model.Trace, 0, len(traceIDs))
	prefixes := make([][]byte, 0, len(traceIDs))

	for _, traceID := range traceIDs {
		prefixes = append(prefixes, createPrimaryKeySeekPrefix(traceID))
	}

	err := r.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		val := []byte{}
		for _, prefix := range prefixes {
			spans := make([]*model.Span, 0, 32) // reduce reallocation requirements by defining some initial length

			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				// Add value to the span store (decode from JSON / defined encoding first)
				// These are in the correct order because of the sorted nature
				item := it.Item()
				val, err := item.ValueCopy(val)
				if err != nil {
					return err
				}

				sp, err := decodeValue(val, item.UserMeta()&encodingTypeBits)
				if err != nil {
					return err
				}
				spans = append(spans, sp)
			}
			if len(spans) > 0 {
				trace := &model.Trace{
					Spans: spans,
				}
				traces = append(traces, trace)
			}
		}
		return nil
	})

	return traces, err
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (r *TraceReader) GetTrace(ctx context.Context, traceID model.TraceID) (*model.Trace, error) {
	traces, err := r.getTraces([]model.TraceID{traceID})
	if err != nil {
		return nil, err
	}
	if len(traces) == 0 {
		return nil, spanstore.ErrTraceNotFound
	}
	if len(traces) == 1 {
		return traces[0], nil
	}

	return nil, ErrInternalConsistencyError
}

// scanTimeRange returns all the Traces found between startTs and endTs
func (r *TraceReader) scanTimeRange(plan *executionPlan) ([]model.TraceID, error) {
	// We need to do a full table scan
	traceKeys := make([][]byte, 0)
	err := r.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		startIndex := []byte{spanKeyPrefix}
		prevTraceID := []byte{}
		for it.Seek(startIndex); it.ValidForPrefix(startIndex); it.Next() {
			item := it.Item()

			key := []byte{}
			key = item.KeyCopy(key)

			timestamp := key[sizeOfTraceID+1 : sizeOfTraceID+1+8]
			traceID := key[1 : sizeOfTraceID+1]

			if bytes.Compare(timestamp, plan.startTimeMin) >= 0 && bytes.Compare(timestamp, plan.startTimeMax) <= 0 {
				if !bytes.Equal(traceID, prevTraceID) {
					if plan.hashOuter != nil {
						trID := bytesToTraceID(traceID)
						if _, exists := plan.hashOuter[trID]; exists {
							traceKeys = append(traceKeys, key)
						}
					} else {
						traceKeys = append(traceKeys, key)
					}
					prevTraceID = traceID
				}
			}
		}

		return nil
	})

	sort.Slice(traceKeys, func(k, h int) bool {
		// This sorts by timestamp to descending order
		return bytes.Compare(traceKeys[k][sizeOfTraceID+1:sizeOfTraceID+1+8], traceKeys[h][sizeOfTraceID+1:sizeOfTraceID+1+8]) > 0
	})

	sizeCount := len(traceKeys)
	if plan.limit > 0 && plan.limit < sizeCount {
		sizeCount = plan.limit
	}
	traceIDs := make([]model.TraceID, sizeCount)

	for i := 0; i < sizeCount; i++ {
		traceIDs[i] = bytesToTraceID(traceKeys[i][1 : sizeOfTraceID+1])
	}

	return traceIDs, err
}

func createPrimaryKeySeekPrefix(traceID model.TraceID) []byte {
	key := make([]byte, 1+sizeOfTraceID)
	key[0] = spanKeyPrefix
	pos := 1
	binary.BigEndian.PutUint64(key[pos:], traceID.High)
	pos += 8
	binary.BigEndian.PutUint64(key[pos:], traceID.Low)

	return key
}

// GetServices fetches the sorted service list that have not expired
func (r *TraceReader) GetServices(ctx context.Context) ([]string, error) {
	return r.cache.GetServices()
}

// GetOperations fetches operations in the service and empty slice if service does not exists
func (r *TraceReader) GetOperations(
	ctx context.Context,
	query spanstore.OperationQueryParameters,
) ([]spanstore.Operation, error) {
	return r.cache.GetOperations(query.ServiceName)
}

// setQueryDefaults alters the query with defaults if certain parameters are not set
func setQueryDefaults(query *spanstore.TraceQueryParameters) {
	if query.NumTraces <= 0 {
		query.NumTraces = defaultNumTraces
	}
}

// serviceQueries parses the query to index seeks which are unique index seeks
func serviceQueries(query *spanstore.TraceQueryParameters, indexSeeks [][]byte) [][]byte {
	if query.ServiceName != "" {
		indexSearchKey := make([]byte, 0, 64) // 64 is a magic guess
		tagQueryUsed := false
		for k, v := range query.Tags {
			tagSearch := []byte(query.ServiceName + k + v)
			tagSearchKey := make([]byte, 0, len(tagSearch)+1)
			tagSearchKey = append(tagSearchKey, tagIndexKey)
			tagSearchKey = append(tagSearchKey, tagSearch...)
			indexSeeks = append(indexSeeks, tagSearchKey)
			tagQueryUsed = true
		}

		if query.OperationName != "" {
			indexSearchKey = append(indexSearchKey, operationNameIndexKey)
			indexSearchKey = append(indexSearchKey, []byte(query.ServiceName+query.OperationName)...)
		} else {
			if !tagQueryUsed { // Tag query already reduces the search set with a serviceName
				indexSearchKey = append(indexSearchKey, serviceNameIndexKey)
				indexSearchKey = append(indexSearchKey, []byte(query.ServiceName)...)
			}
		}

		if len(indexSearchKey) > 0 {
			indexSeeks = append(indexSeeks, indexSearchKey)
		}
	}
	return indexSeeks
}

// indexSeeksToTraceIDs does the index scanning against badger based on the parsed index queries
func (r *TraceReader) indexSeeksToTraceIDs(plan *executionPlan, indexSeeks [][]byte) ([]model.TraceID, error) {

	for i := len(indexSeeks) - 1; i > 0; i-- {
		indexResults, err := r.scanIndexKeys(indexSeeks[i], plan)
		if err != nil {
			return nil, err
		}

		sort.Slice(indexResults, func(k, h int) bool {
			return bytes.Compare(indexResults[k], indexResults[h]) < 0
		})

		// Same traceID can be returned multiple times, but always in sorted order so checking the previous key is enough
		prevTraceID := []byte{}
		innerIDs := make([][]byte, 0, len(indexSeeks))
		for j := 0; j < len(indexResults); j++ {
			traceID := indexResults[j]
			if !bytes.Equal(prevTraceID, traceID) {
				innerIDs = append(innerIDs, traceID)
				prevTraceID = traceID
			}
		}

		// Merge-join current results
		if plan.mergeOuter == nil {
			plan.mergeOuter = innerIDs
		} else {
			plan.mergeOuter = mergeJoinIds(plan.mergeOuter, innerIDs)
		}
	}

	// Last scan should get us in correct timestamp order
	ids, err := r.scanIndexKeys(indexSeeks[0], plan)
	if err != nil {
		return nil, err
	}

	if plan.mergeOuter != nil {
		// Build hash of the current merged data
		plan.hashOuter = buildHash(plan, plan.mergeOuter)
		plan.mergeOuter = nil
	} else {
		// We filter the last elements
		plan.hashOuter = buildHash(plan, ids)
	}

	traceIDs := filterIDs(plan, ids)
	return traceIDs, nil
}

func filterIDs(plan *executionPlan, innerIDs [][]byte) []model.TraceID {
	traces := make([]model.TraceID, 0, plan.limit)

	items := 0
	for i := 0; i < len(innerIDs); i++ {
		trID := bytesToTraceID(innerIDs[i])

		if _, found := plan.hashOuter[trID]; found {
			traces = append(traces, trID)
			delete(plan.hashOuter, trID) // Prevent duplicate add
			items++
		}

		if items == plan.limit {
			return traces
		}
	}

	return traces
}

func bytesToTraceID(key []byte) model.TraceID {
	return model.TraceID{
		High: binary.BigEndian.Uint64(key[:8]),
		Low:  binary.BigEndian.Uint64(key[8:sizeOfTraceID]),
	}
}

func buildHash(plan *executionPlan, outerIDs [][]byte) map[model.TraceID]struct{} {
	var empty struct{}

	hashed := make(map[model.TraceID]struct{})
	for i := 0; i < len(outerIDs); i++ {
		trID := bytesToTraceID(outerIDs[i])

		if plan.hashOuter != nil {
			if _, exists := plan.hashOuter[trID]; exists {
				hashed[trID] = empty
				delete(plan.hashOuter, trID) // Filter duplications
			}
		} else {
			hashed[trID] = empty
		}
	}

	return hashed
}

// durationQueries checks non unique index of durations and returns a map for further filtering purposes
func (r *TraceReader) durationQueries(plan *executionPlan, query *spanstore.TraceQueryParameters) map[model.TraceID]struct{} {
	durMax := uint64(model.DurationAsMicroseconds(query.DurationMax))
	durMin := uint64(model.DurationAsMicroseconds(query.DurationMin))

	startKey := make([]byte, 1+8)
	endKey := make([]byte, 1+8)

	startKey[0] = durationIndexKey
	endKey[0] = durationIndexKey

	if query.DurationMax == 0 {
		// Set MAX to infinite, if Min is missing, 0 is a fine search result for us
		durMax = math.MaxUint64
	}
	binary.BigEndian.PutUint64(endKey[1:], durMax)
	binary.BigEndian.PutUint64(startKey[1:], durMin)

	// This is not unique index result - same TraceID can be matched from multiple spans
	indexResults, _ := r.scanRangeIndex(plan, startKey, endKey)
	hashFilter := make(map[model.TraceID]struct{})
	var value struct{}
	for _, k := range indexResults {
		key := k[len(k)-sizeOfTraceID:]
		id := model.TraceID{
			High: binary.BigEndian.Uint64(key[:8]),
			Low:  binary.BigEndian.Uint64(key[8:]),
		}
		if _, exists := hashFilter[id]; !exists {
			hashFilter[id] = value
		}
	}

	return hashFilter
}

func mergeJoinIds(left, right [][]byte) [][]byte {
	// len(left) or len(right) is the maximum, whichever is the smallest
	allocateSize := len(left)
	if len(right) < allocateSize {
		allocateSize = len(right)
	}

	merged := make([][]byte, 0, allocateSize)

	lMax := len(left) - 1
	rMax := len(right) - 1
	for r, l := 0, 0; r <= rMax && l <= lMax; {
		switch bytes.Compare(left[l], right[r]) {
		case 0:
			// Left matches right - merge
			merged = append(merged, left[l])
			// Advance both
			l++
			r++
		case 1:
			// left > right, increase right one
			r++
		case -1:
			// left < right, increase left one
			l++
		}
	}
	return merged
}

// FindTraces retrieves traces that match the traceQuery
func (r *TraceReader) FindTraces(ctx context.Context, query *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	keys, err := r.FindTraceIDs(ctx, query)
	if err != nil {
		return nil, err
	}

	return r.getTraces(keys)
}

// FindTraceIDs retrieves only the TraceIDs that match the traceQuery, but not the trace data
func (r *TraceReader) FindTraceIDs(ctx context.Context, query *spanstore.TraceQueryParameters) ([]model.TraceID, error) {
	// Validate and set query defaults which were not defined
	if err := validateQuery(query); err != nil {
		return nil, err
	}

	setQueryDefaults(query)

	// Find matches using indexes that are using service as part of the key
	indexSeeks := make([][]byte, 0, 1)
	indexSeeks = serviceQueries(query, indexSeeks)

	startStampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(startStampBytes, model.TimeAsEpochMicroseconds(query.StartTimeMin))

	endStampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(endStampBytes, model.TimeAsEpochMicroseconds(query.StartTimeMax))

	plan := &executionPlan{
		startTimeMin: startStampBytes,
		startTimeMax: endStampBytes,
		limit:        query.NumTraces,
	}

	if query.DurationMax != 0 || query.DurationMin != 0 {
		plan.hashOuter = r.durationQueries(plan, query)
	}

	if len(indexSeeks) > 0 {
		keys, err := r.indexSeeksToTraceIDs(plan, indexSeeks)
		if err != nil {
			return nil, err
		}

		return keys, nil
	}

	return r.scanTimeRange(plan)
}

// validateQuery returns an error if certain restrictions are not met
func validateQuery(p *spanstore.TraceQueryParameters) error {
	if p == nil {
		return ErrMalformedRequestObject
	}
	if p.ServiceName == "" && len(p.Tags) > 0 {
		return ErrServiceNameNotSet
	}
	if p.ServiceName == "" && p.OperationName != "" {
		return ErrServiceNameNotSet
	}
	if p.StartTimeMin.IsZero() || p.StartTimeMax.IsZero() {
		return ErrStartAndEndTimeNotSet
	}
	if !p.StartTimeMax.IsZero() && p.StartTimeMax.Before(p.StartTimeMin) {
		return ErrStartTimeMinGreaterThanMax
	}
	if p.DurationMin != 0 && p.DurationMax != 0 && p.DurationMin > p.DurationMax {
		return ErrDurationMinGreaterThanMax
	}
	return nil
}

// scanIndexKeys scans the time range for index keys matching the given prefix.
func (r *TraceReader) scanIndexKeys(indexKeyValue []byte, plan *executionPlan) ([][]byte, error) {
	indexResults := make([][]byte, 0)

	err := r.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Don't fetch values since we're only interested in the keys
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		// Create starting point for sorted index scan
		startIndex := make([]byte, len(indexKeyValue)+8+1)
		startIndex[len(startIndex)-1] = 0xFF
		copy(startIndex, indexKeyValue)
		copy(startIndex[len(indexKeyValue):], plan.startTimeMax)

		for it.Seek(startIndex); scanFunction(it, indexKeyValue, plan.startTimeMin); it.Next() {
			item := it.Item()

			// ScanFunction is a prefix scanning (since we could have for example service1 & service12)
			// Now we need to match only the exact key if we want to add it
			timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // timestamp is stored with 8 bytes
			if bytes.Equal(indexKeyValue, it.Item().Key()[:timestampStartIndex]) {
				traceIDBytes := item.Key()[len(item.Key())-sizeOfTraceID:]

				traceIDCopy := make([]byte, sizeOfTraceID)
				copy(traceIDCopy, traceIDBytes)
				indexResults = append(indexResults, traceIDCopy)
			}
		}
		return nil
	})

	return indexResults, err
}

// scanFunction compares the index name as well as the time range in the index key
func scanFunction(it *badger.Iterator, indexPrefix []byte, timeBytesEnd []byte) bool {
	if it.Valid() {
		// We can't use the indexPrefix length, because we might have the same prefixValue for non-matching cases also
		timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // timestamp is stored with 8 bytes
		timestamp := it.Item().Key()[timestampStartIndex : timestampStartIndex+8]
		timestampInRange := bytes.Compare(timeBytesEnd, timestamp) <= 0

		// Check length as well to prevent theoretical case where timestamp might match with wrong index key
		if len(it.Item().Key()) != len(indexPrefix)+24 {
			return false
		}

		return bytes.HasPrefix(it.Item().Key()[:timestampStartIndex], indexPrefix) && timestampInRange
	}
	return false
}

// scanRangeIndex scans the time range for index keys matching the given prefix.
func (r *TraceReader) scanRangeIndex(plan *executionPlan, indexStartValue []byte, indexEndValue []byte) ([][]byte, error) {
	indexResults := make([][]byte, 0)

	err := r.store.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false // Don't fetch values since we're only interested in the keys
		it := txn.NewIterator(opts)
		defer it.Close()

		// Create starting point for sorted index scan
		startIndex := make([]byte, len(indexStartValue)+len(plan.startTimeMin))
		copy(startIndex, indexStartValue)
		copy(startIndex[len(indexStartValue):], plan.startTimeMin)

		for it.Seek(startIndex); scanRangeFunction(it, indexEndValue); it.Next() {
			item := it.Item()

			// ScanFunction is a prefix scanning (since we could have for example service1 & service12)
			// Now we need to match only the exact key if we want to add it
			timestampStartIndex := len(it.Item().Key()) - (sizeOfTraceID + 8) // timestamp is stored with 8 bytes
			timestamp := it.Item().Key()[timestampStartIndex : timestampStartIndex+8]
			if bytes.Compare(timestamp, plan.startTimeMax) <= 0 {
				key := make([]byte, len(item.Key()))
				copy(key, item.Key())
				indexResults = append(indexResults, key)
			}
		}
		return nil
	})
	return indexResults, err
}

// scanRangeFunction seeks until the index end has been reached
func scanRangeFunction(it *badger.Iterator, indexEndValue []byte) bool {
	if it.Valid() {
		compareSlice := it.Item().Key()[:len(indexEndValue)]
		return bytes.Compare(indexEndValue, compareSlice) >= 0
	}
	return false
}
