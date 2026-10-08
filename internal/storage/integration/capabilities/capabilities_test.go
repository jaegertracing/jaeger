// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAttributeOrderingOutcome checks that exactly one of the three attribute-ordering cases
// runs for every deployment the constructors and the two modifiers describe.
func TestAttributeOrderingOutcome(t *testing.T) {
	tests := []struct {
		name string
		caps Capabilities
		runs string
	}{
		{name: "Elasticsearch", caps: Elasticsearch(), runs: attributeOrderingTest},
		{name: "OpenSearch", caps: OpenSearch(), runs: attributeOrderingTest},
		{name: "Elasticsearch without numeric attributes", caps: Elasticsearch().WithoutNumericAttributes(), runs: attributeRefusedTest},
		{name: "OpenSearch without numeric attributes", caps: OpenSearch().WithoutNumericAttributes(), runs: attributeRefusedTest},
		{name: "Elasticsearch with the mapping over older indices", caps: Elasticsearch().WithNumericAttributesNotYetIndexed(), runs: attributeUnindexedTest},
		{name: "modifiers replace each other", caps: Elasticsearch().WithoutNumericAttributes().WithNumericAttributesNotYetIndexed(), runs: attributeUnindexedTest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			skipped := test.caps.SkipList()
			for _, outcome := range attributeOrderingTests {
				count := 0
				for _, name := range skipped {
					if name == outcome {
						count++
					}
				}
				if outcome == test.runs {
					assert.Zero(t, count, "%s runs", outcome)
				} else {
					assert.Equal(t, 1, count, "%s is skipped once", outcome)
				}
			}
		})
	}
}

// capabilityModifiers lists every exported method that derives one capability set from another.
var capabilityModifiers = map[string]func(Capabilities) Capabilities{
	"WithoutPagination":                   Capabilities.WithoutPagination,
	"WithoutTraceIDPagination":            Capabilities.WithoutTraceIDPagination,
	"WithoutSpanSearch":                   Capabilities.WithoutSpanSearch,
	"WithoutSpanSorting":                  Capabilities.WithoutSpanSorting,
	"WithoutSpanAttributeOrdering":        Capabilities.WithoutSpanAttributeOrdering,
	"WithOTLPEncoding":                    Capabilities.WithOTLPEncoding,
	"WithoutNumericAttributes":            Capabilities.WithoutNumericAttributes,
	"WithNumericAttributesNotYetIndexed":  Capabilities.WithNumericAttributesNotYetIndexed,
	"WithoutTextAttributeOrderingRefusal": Capabilities.WithoutTextAttributeOrderingRefusal,
	"WithoutAttributeRefusal":             Capabilities.WithoutAttributeRefusal,
	"WithoutUnindexedFieldRefusal":        Capabilities.WithoutUnindexedFieldRefusal,
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

func TestCapabilityModifiersCopyTheSkipList(t *testing.T) {
	// A modifier must change a copy of the receiver's list, keeping unrelated entries.
	// The base is built with spare capacity, so a modifier that appended in place would
	// hand the derived value a view of the base's array, and writing into that spare slot
	// afterwards would show up in it.
	for name, modify := range capabilityModifiers {
		t.Run(name, func(t *testing.T) {
			base := Capabilities{skipList: append(make([]string, 0, 8),
				"existing", findTraceSummariesTest, scopeAttributesTest, linkAttributesTest)}
			before := slices.Clone(base.SkipList())
			derived := modify(base)
			base.skipList = append(base.skipList, "sentinel")
			assert.Contains(t, derived.SkipList(), "existing")
			assert.NotContains(t, derived.SkipList(), "sentinel")
			assert.NotEqual(t, before, derived.SkipList(), "the modifier left the list as it was")
		})
	}
}

func TestCapabilityModifiersAreAllListed(t *testing.T) {
	// A new modifier that is not in capabilityModifiers escapes the copy test, so every exported
	// method that derives one Capabilities from another has to be listed there.
	capabilitiesType := reflect.TypeOf(Capabilities{})
	for i := range capabilitiesType.NumMethod() {
		method := capabilitiesType.Method(i)
		isModifier := method.Type.NumIn() == 1 && method.Type.NumOut() == 1 && method.Type.Out(0) == capabilitiesType
		if isModifier {
			assert.Contains(t, capabilityModifiers, method.Name)
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

func TestPagingDropsTiedSpans(t *testing.T) {
	// Only a backend whose span search runs without the _id tie-breaker declares the drop, and a
	// constructor that chains an opt-out must carry the flag through.
	assert.True(t, Elasticsearch().PagingDropsTiedSpans())
	assert.True(t, Elasticsearch().WithoutNumericAttributes().PagingDropsTiedSpans())
	assert.True(t, ElasticsearchSmokeTest().PagingDropsTiedSpans())
	assert.False(t, OpenSearch().PagingDropsTiedSpans())
	assert.False(t, Memory().PagingDropsTiedSpans())
}
