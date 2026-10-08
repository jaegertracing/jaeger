// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

import (
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAttributeComparisonOutcome checks the outcome each deployment declares for an ordered
// comparison on an attribute: the constructors expect the comparison answered, and the two modifiers each name
// one of the other outcomes, the later one winning.
func TestAttributeComparisonOutcome(t *testing.T) {
	tests := []struct {
		name    string
		caps    Capabilities
		outcome AttributeComparison
	}{
		{name: "Elasticsearch", caps: Elasticsearch(), outcome: AttributeComparisonNumeric},
		{name: "OpenSearch", caps: OpenSearch(), outcome: AttributeComparisonNumeric},
		{name: "Memory", caps: Memory(), outcome: AttributeComparisonNumeric},
		{name: "Elasticsearch without numeric attributes", caps: Elasticsearch().WithoutNumericAttributes(), outcome: AttributeComparisonRefused},
		{name: "OpenSearch without numeric attributes", caps: OpenSearch().WithoutNumericAttributes(), outcome: AttributeComparisonRefused},
		{name: "Elasticsearch with the mapping over older indices", caps: Elasticsearch().WithNumericAttributesEnabledAtUpgrade(), outcome: AttributeComparisonMissesOlderIndices},
		{name: "modifiers replace each other", caps: Elasticsearch().WithoutNumericAttributes().WithNumericAttributesEnabledAtUpgrade(), outcome: AttributeComparisonMissesOlderIndices},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.outcome, test.caps.AttributeComparison())
			assert.NotContains(t, test.caps.SkipList(), attributeComparisonTest, "the outcome is asserted, not skipped")
		})
	}
}

// optOuts is every exported method that adds to the skip list; TestOptOutsAreAllListed fails when
// a new one is not added here.
var optOuts = map[string]func(Capabilities) Capabilities{
	"WithoutPagination":                 Capabilities.WithoutPagination,
	"WithoutTraceIDPagination":          Capabilities.WithoutTraceIDPagination,
	"WithoutSpanSearch":                 Capabilities.WithoutSpanSearch,
	"WithoutSpanSorting":                Capabilities.WithoutSpanSorting,
	"WithoutSpanAttributeOrdering":      Capabilities.WithoutSpanAttributeOrdering,
	"WithoutUnindexedLevelRefusal":      Capabilities.WithoutUnindexedLevelRefusal,
	"WithoutLevelRefusal":               Capabilities.WithoutLevelRefusal,
	"WithoutUnevaluatedOperatorRefusal": Capabilities.WithoutUnevaluatedOperatorRefusal,
	"WithoutOperatorRefusal":            Capabilities.WithoutOperatorRefusal,
	"WithoutFilterRefusals":             Capabilities.WithoutFilterRefusals,
}

// outcomeModifiers is every exported method that sets a typed field instead of adding to the
// skip list, so TestOptOutsAreAllListed can tell a new method of either kind from one that was
// forgotten.
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

func TestOutcomeModifiersLeaveTheSkipListAlone(t *testing.T) {
	for name, modify := range outcomeModifiers {
		t.Run(name, func(t *testing.T) {
			base := Capabilities{skipList: []string{"existing"}}
			assert.Equal(t, base.SkipList(), modify(base).SkipList())
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

func TestTraceStateRefused(t *testing.T) {
	// Only the readers whose schema has no place for span.traceState declare the refusal, and
	// a constructor that chains a modifier must carry the flag through.
	assert.True(t, Elasticsearch().TraceStateRefused())
	assert.True(t, Elasticsearch().WithoutNumericAttributes().TraceStateRefused())
	assert.True(t, ElasticsearchSmokeTest().TraceStateRefused())
	assert.True(t, OpenSearch().TraceStateRefused())
	assert.True(t, ClickHouse().TraceStateRefused())
	assert.True(t, ClickHouseE2E().TraceStateRefused())
	assert.False(t, Memory().TraceStateRefused())
	assert.False(t, GRPC().TraceStateRefused())
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
