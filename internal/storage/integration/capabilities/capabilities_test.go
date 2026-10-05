// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

import (
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

// optOuts is every exported opt-out, so that each is checked once and called once.
var optOuts = map[string]func(Capabilities) Capabilities{
	"WithoutPagination":                   Capabilities.WithoutPagination,
	"WithoutTraceIDPagination":            Capabilities.WithoutTraceIDPagination,
	"WithoutSpanSearch":                   Capabilities.WithoutSpanSearch,
	"WithoutSpanSorting":                  Capabilities.WithoutSpanSorting,
	"WithoutSpanAttributeOrdering":        Capabilities.WithoutSpanAttributeOrdering,
	"WithoutNumericAttributes":            Capabilities.WithoutNumericAttributes,
	"WithNumericAttributesNotYetIndexed":  Capabilities.WithNumericAttributesNotYetIndexed,
	"WithoutUnindexedLevelRefusal":        Capabilities.WithoutUnindexedLevelRefusal,
	"WithoutLevelRefusal":                 Capabilities.WithoutLevelRefusal,
	"WithoutUnevaluatedOperatorRefusal":   Capabilities.WithoutUnevaluatedOperatorRefusal,
	"WithoutOperatorRefusal":              Capabilities.WithoutOperatorRefusal,
	"WithoutTextAttributeOrderingRefusal": Capabilities.WithoutTextAttributeOrderingRefusal,
	"WithoutAttributeRefusal":             Capabilities.WithoutAttributeRefusal,
	"WithoutUnindexedFieldRefusal":        Capabilities.WithoutUnindexedFieldRefusal,
	"WithoutFilterRefusals":               Capabilities.WithoutFilterRefusals,
}

// backends is every backend's declaration, so that each is checked once and called once.
var backends = map[string]func() Capabilities{
	"Memory":                  Memory,
	"GRPC":                    GRPC,
	"Cassandra":               Cassandra,
	"ClickHouse":              ClickHouse,
	"Badger":                  Badger,
	"Elasticsearch":           Elasticsearch,
	"ElasticsearchSmokeTest":  ElasticsearchSmokeTest,
	"OpenSearch":              OpenSearch,
	"Kafka":                   Kafka,
	"E2EWithoutNativeFilters": E2EWithoutNativeFilters,
}

func TestOptOutsCopyTheSkipList(t *testing.T) {
	// An opt-out must add its entries to a copy of the receiver's list. The base is built with
	// spare capacity, so an opt-out that appended in place would hand the derived value a view
	// of the base's array, and writing into that spare slot afterwards would show up in it.
	for name, optOut := range optOuts {
		t.Run(name, func(t *testing.T) {
			base := Capabilities{skipList: make([]string, 0, 8)}
			derived := optOut(base)
			_ = append(base.skipList, "sentinel")
			assert.NotContains(t, derived.SkipList(), "sentinel")
			assert.NotEmpty(t, derived.SkipList(), "the opt-out added nothing")
		})
	}
}

func TestSkipListsHaveNoDuplicates(t *testing.T) {
	// A repeated entry is harmless to skipIfNeeded but marks a list that was edited without
	// being read.
	lists := map[string]Capabilities{}
	for name, backend := range backends {
		lists[name] = backend()
	}
	for name, optOut := range optOuts {
		lists[name] = optOut(Capabilities{})
	}
	for name, caps := range lists {
		t.Run(name, func(t *testing.T) {
			list := caps.SkipList()
			assert.Len(t, slices.Compact(slices.Sorted(slices.Values(list))), len(list), "duplicate entries in %v", list)
		})
	}
}
