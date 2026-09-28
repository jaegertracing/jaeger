// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

import (
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

func TestFilterRefusalOptOuts(t *testing.T) {
	t.Run("WithoutUnindexedLevelRefusal", func(t *testing.T) {
		caps := Capabilities{}.WithoutUnindexedLevelRefusal()
		assert.Contains(t, caps.SkipList(), levelRefusedTest)
	})

	t.Run("WithoutLevelRefusal", func(t *testing.T) {
		caps := Capabilities{}.WithoutLevelRefusal()
		assert.Contains(t, caps.SkipList(), levelRefusedTest)
	})

	t.Run("WithoutUnevaluatedOperatorRefusal", func(t *testing.T) {
		caps := Capabilities{}.WithoutUnevaluatedOperatorRefusal()
		assert.Contains(t, caps.SkipList(), operatorRefusedTest)
	})

	t.Run("WithoutOperatorRefusal", func(t *testing.T) {
		caps := Capabilities{}.WithoutOperatorRefusal()
		assert.Contains(t, caps.SkipList(), operatorRefusedTest)
	})

	t.Run("WithoutTextAttributeOrderingRefusal", func(t *testing.T) {
		caps := Capabilities{}.WithoutTextAttributeOrderingRefusal()
		assert.Contains(t, caps.SkipList(), attributeRefusedTest)
	})

	t.Run("WithoutAttributeRefusal", func(t *testing.T) {
		caps := Capabilities{}.WithoutAttributeRefusal()
		assert.Contains(t, caps.SkipList(), attributeRefusedTest)
	})

	t.Run("WithoutFilterRefusals", func(t *testing.T) {
		caps := Capabilities{}.WithoutFilterRefusals()
		assert.Contains(t, caps.SkipList(), levelRefusedTest)
		assert.Contains(t, caps.SkipList(), operatorRefusedTest)
		assert.Contains(t, caps.SkipList(), attributeRefusedTest)
	})

	t.Run("WithoutSpanAttributeOrdering", func(t *testing.T) {
		caps := Capabilities{}.WithoutSpanAttributeOrdering()
		assert.Contains(t, caps.SkipList(), spanAttributeOrderingTest)
	})

	t.Run("Memory capabilities", func(t *testing.T) {
		caps := Memory()
		assert.NotContains(t, caps.SkipList(), structuredFilterTest)
		assert.Contains(t, caps.SkipList(), levelRefusedTest)
		assert.Contains(t, caps.SkipList(), operatorRefusedTest)
		assert.Contains(t, caps.SkipList(), attributeRefusedTest)
		assert.Contains(t, caps.SkipList(), findTraceSummariesTest)
	})

	t.Run("GRPC capabilities", func(t *testing.T) {
		caps := GRPC()
		assert.NotContains(t, caps.SkipList(), structuredFilterTest)
		assert.Contains(t, caps.SkipList(), levelRefusedTest)
		assert.Contains(t, caps.SkipList(), operatorRefusedTest)
		assert.Contains(t, caps.SkipList(), attributeRefusedTest)
		assert.Contains(t, caps.SkipList(), findTraceSummariesTest)
	})
}
