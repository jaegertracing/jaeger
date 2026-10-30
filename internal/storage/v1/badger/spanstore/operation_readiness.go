// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import "github.com/dgraph-io/badger/v4"

// OperationIndexReadiness describes persisted span-kind coverage for one database.
type OperationIndexReadiness struct {
	Ready          bool
	LegacyEntries  int
	PartialEntries int
	InvalidEntries int
	Err            error
}

// OperationIndexReadiness returns the startup check, which is not reused across restarts.
func (r *TraceReader) OperationIndexReadiness() OperationIndexReadiness {
	return r.cache.readiness
}

func (r *TraceReader) prefillOperationIndex() {
	status := OperationIndexReadiness{}
	services, err := r.preloadServices()
	if err == nil {
		for _, service := range services {
			if err = r.preloadOperations(service); err != nil {
				break
			}
		}
	}
	if err != nil {
		status.Err = err
		r.cache.readiness = status
		return
	}
	status.Err = r.store.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte{operationNameIndexKey}
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			value, err := it.Item().ValueCopy(nil)
			if err != nil {
				return err
			}
			kinds, err := decodeOperationKinds(value)
			switch {
			case err != nil:
				status.InvalidEntries++
			case len(value) == 0:
				status.LegacyEntries++
			case kinds.legacy:
				status.PartialEntries++
			default:
				continue
			}
		}
		return nil
	})
	status.Ready = status.Err == nil && status.LegacyEntries == 0 && status.PartialEntries == 0 && status.InvalidEntries == 0
	r.cache.readiness = status
}
