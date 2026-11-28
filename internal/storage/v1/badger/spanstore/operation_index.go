// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/dgraph-io/badger/v4"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

var operationIndexGate = featuregate.GlobalRegistry().MustRegister(
	"jaeger.badger.operationIndex",
	featuregate.StageAlpha,
	featuregate.WithRegisterDescription("Return and filter Badger operations by span kind after checking persisted index coverage."),
)

const (
	operationIndexVersion = 1
	operationIndexLegacy  = 1
)

type operationKinds struct {
	legacy bool
	kinds  []string
}

func (k operationKinds) encode() []byte {
	value := []byte{operationIndexVersion, 0}
	if k.legacy {
		value[1] = operationIndexLegacy
	}
	for _, kind := range k.kinds {
		// Badger's value-size limit is smaller than the uint32 length field.
		//nolint:gosec // G115
		value = binary.BigEndian.AppendUint32(value, uint32(len(kind)))
		value = append(value, kind...)
	}
	return value
}

func decodeOperationKinds(value []byte) (operationKinds, error) {
	if len(value) == 0 {
		return operationKinds{legacy: true}, nil
	}
	if len(value) < 2 || value[0] != operationIndexVersion || value[1]&^byte(operationIndexLegacy) != 0 {
		return operationKinds{}, errors.New("invalid operation index header")
	}
	result := operationKinds{legacy: value[1]&operationIndexLegacy != 0}
	for remaining := value[2:]; len(remaining) > 0; {
		if len(remaining) < 4 {
			return operationKinds{}, errors.New("truncated operation kind length")
		}
		length := uint64(binary.BigEndian.Uint32(remaining))
		remaining = remaining[4:]
		if length > uint64(len(remaining)) {
			return operationKinds{}, errors.New("truncated operation kind")
		}
		kind := string(remaining[:length])
		if len(result.kinds) > 0 && result.kinds[len(result.kinds)-1] >= kind {
			return operationKinds{}, errors.New("operation kinds must be distinct and sorted")
		}
		result.kinds = append(result.kinds, kind)
		remaining = remaining[length:]
	}
	if len(result.kinds) == 0 {
		return operationKinds{}, errors.New("operation index has no kinds")
	}
	return result, nil
}

func spanKind(span *model.Span) string {
	for _, tag := range span.Tags {
		if tag.Key == model.SpanKindKey {
			return tag.AsString()
		}
	}
	return ""
}

// laterExpiry treats zero as the absence of expiry, as Badger does.
func laterExpiry(a, b uint64) uint64 {
	if a == 0 || b == 0 {
		return 0
	}
	return max(a, b)
}

func mergeOperationIndex(txn *badger.Txn, key []byte, kind string, expiry uint64) (operationKinds, uint64, error) {
	kinds := operationKinds{}
	item, err := txn.Get(key)
	if err == nil {
		value, err := item.ValueCopy(nil)
		if err != nil {
			return kinds, 0, err
		}
		kinds, err = decodeOperationKinds(value)
		if err != nil {
			return kinds, 0, fmt.Errorf("decode operation index: %w", err)
		}
		expiry = laterExpiry(expiry, item.ExpiresAt())
	} else if !errors.Is(err, badger.ErrKeyNotFound) {
		return kinds, 0, err
	}
	if index, found := slices.BinarySearch(kinds.kinds, kind); !found {
		kinds.kinds = slices.Insert(kinds.kinds, index, kind)
	}
	return kinds, expiry, nil
}

func mergeServiceExpiry(txn *badger.Txn, key []byte, expiry uint64) (uint64, error) {
	item, err := txn.Get(key)
	if err == nil {
		return laterExpiry(expiry, item.ExpiresAt()), nil
	}
	if errors.Is(err, badger.ErrKeyNotFound) {
		return expiry, nil
	}
	return 0, err
}
