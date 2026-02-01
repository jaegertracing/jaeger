// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"

	"github.com/dgraph-io/badger/v4"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

var _ tracestore.Reader = (*Reader)(nil)

const (
	// operationNameIndexKey (0x82) is the prefix for the operations index.
	operationNameIndexKey byte = 0x82

	// sizeOfTraceID is 16 bytes for a TraceID.
	sizeOfTraceID = 16
)

// Reader implements the tracestore.Reader interface for Badger.
type Reader struct {
	db *badger.DB
}

// NewReader creates a new Reader instance.
func NewReader(db *badger.DB) *Reader {
	return &Reader{
		db: db,
	}
}

// GetOperations returns all operation names for a given service.
func (r *Reader) GetOperations(_ context.Context, query tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	var operations []tracestore.Operation

	err := r.db.View(func(txn *badger.Txn) error {
		// Create the search prefix: [0x82][ServiceName]
		prefix := make([]byte, len(query.ServiceName)+1)
		prefix[0] = operationNameIndexKey
		copy(prefix[1:], query.ServiceName)

		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		uniqueOps := make(map[string]struct{})
		for it.Seek(prefix); it.ValidForPrefix((prefix)); it.Next() {
			key := it.Item().Key()

			// Key layout: [Prefix(1)][Service(N)][Operation(M)][Time(8)][TraceID(16)]
			opNameStart := 1 + len(query.ServiceName)
			opNameEnd := len(key) - (8 + sizeOfTraceID)

			if opNameEnd > opNameStart {
				opName := string(key[opNameStart:opNameEnd])
				if _, exists := uniqueOps[opName]; !exists {
					uniqueOps[opName] = struct{}{}
					operations = append(operations, tracestore.Operation{
						Name: opName,
					})
				}
			}
		}
		return nil
	})
	return operations, err
}

// Stubs
func (*Reader) GetServices(context.Context) ([]string, error) {
	return nil, nil
}

func (*Reader) GetTraces(context.Context, ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(_ func([]ptrace.Traces, error) bool) {}
}

func (*Reader) FindTraces(context.Context, tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(_ func([]ptrace.Traces, error) bool) {}
}

func (*Reader) FindTraceIDs(context.Context, tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	return func(_ func([]tracestore.FoundTraceID, error) bool) {}
}
