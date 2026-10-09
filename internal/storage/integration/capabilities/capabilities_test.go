// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// optOuts is every exported method that adds to the skip list; TestOptOutsAreAllListed fails when
// a new one is not added here.
var optOuts = map[string]func(Capabilities) Capabilities{
	"WithoutPagination":            Capabilities.WithoutPagination,
	"WithoutTraceIDPagination":     Capabilities.WithoutTraceIDPagination,
	"WithoutSpanSearch":            Capabilities.WithoutSpanSearch,
	"WithoutSpanSorting":           Capabilities.WithoutSpanSorting,
	"WithoutSpanAttributeOrdering": Capabilities.WithoutSpanAttributeOrdering,
}

// outcomeModifiers is every exported method that sets a typed field instead of adding to the
// skip list, so TestOptOutsAreAllListed can tell a new method of either kind from one that was
// forgotten. They are not in optOuts because they leave the skip list alone.
var outcomeModifiers = map[string]func(Capabilities) Capabilities{
	"WithoutNumericAttributes":              Capabilities.WithoutNumericAttributes,
	"WithNumericAttributesEnabledAtUpgrade": Capabilities.WithNumericAttributesEnabledAtUpgrade,
}

// backends is the backend declarations, so that each is checked once and called once.
var backends = map[string]func() Capabilities{
	"Memory":                  Memory,
	"GRPC":                    GRPC,
	"Cassandra":               Cassandra,
	"ClickHouse":              ClickHouse,
	"ClickHouseE2E":           ClickHouseE2E,
	"Badger":                  Badger,
	"Elasticsearch":           Elasticsearch,
	"ElasticsearchSmokeTest":  ElasticsearchSmokeTest,
	"OpenSearch":              OpenSearch,
	"Kafka":                   Kafka,
	"E2EWithoutNativeFilters": E2EWithoutNativeFilters,
}

func TestOptOutsCopyTheSkipList(t *testing.T) {
	// An opt-out must add its entries to a copy of the receiver's list, keeping what is already
	// there. The base is built with spare capacity, so an opt-out that appended in place would
	// hand the derived value a view of the base's array, and writing into that spare slot
	// afterwards would show up in it.
	for name, optOut := range optOuts {
		t.Run(name, func(t *testing.T) {
			base := Capabilities{skipList: append(make([]string, 0, 8), "existing")}
			derived := optOut(base)
			base.skipList = append(base.skipList, "sentinel")
			assert.Contains(t, derived.SkipList(), "existing")
			assert.NotContains(t, derived.SkipList(), "sentinel")
			assert.Greater(t, len(derived.SkipList()), 1, "the opt-out left the list as it was")
		})
	}
}

func TestOptOutsAreAllListed(t *testing.T) {
	// A new opt-out that is not in optOuts escapes TestOptOutsCopyTheSkipList, so every exported
	// method that derives one Capabilities from another has to be listed there or in
	// outcomeModifiers.
	capabilitiesType := reflect.TypeOf(Capabilities{})
	for i := range capabilitiesType.NumMethod() {
		method := capabilitiesType.Method(i)
		isOptOut := method.Type.NumIn() == 1 && method.Type.NumOut() == 1 && method.Type.Out(0) == capabilitiesType
		if isOptOut {
			_, listed := optOuts[method.Name]
			_, modifies := outcomeModifiers[method.Name]
			assert.True(t, listed || modifies, "%s is in neither optOuts nor outcomeModifiers", method.Name)
		}
	}
}

func TestBackendSkipListsHaveNoDuplicates(t *testing.T) {
	// A repeated entry is harmless to skipIfNeeded but marks a list that was edited without
	// being read.
	for name, backend := range backends {
		t.Run(name, func(t *testing.T) {
			list := backend().SkipList()
			assert.Len(t, slices.Compact(slices.Sorted(slices.Values(list))), len(list), "duplicate entries in %v", list)
		})
	}
}
