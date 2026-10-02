// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

import "slices"

const (
	scopeAttributesTest    = "Scope_Attributes"
	linkAttributesTest     = "Link_Attributes"
	findTraceSummariesTest = "FindTraceSummaries"
	structuredFilterTest   = "FindTracesWithFilter"
	paginationTest         = "Pagination"
	spanOrderingTest       = "SpanOrdering"
	// spanAttributeOrderingTest orders spans by an attribute, which no backend supports yet: the
	// ordering contract admits only intrinsic span fields. Every backend that runs the ordering
	// battery lists it, so the incomplete functionality is recorded here rather than silently
	// untested. Remove the entry from a backend once it orders by attributes.
	spanAttributeOrderingTest = "SpanOrdering/Attributes"
	traceIDPaginationTest     = "Pagination/TraceIDs"
	spanPaginationTest        = "Pagination/Spans"
	summaryPaginationTest     = "Pagination/TraceSummaries"

	// The battery has three cases for ordering an attribute, of which exactly one runs for any
	// deployment: it is answered where the indices carry the typed-attribute mapping (RFC 0015),
	// refused where the mapping is not configured, and answered with nothing where the mapping is
	// configured but the indices were created before it. Elasticsearch and OpenSearch run the first
	// by default, because the suites that run the battery configure the mapping;
	// WithoutNumericAttributes and WithNumericAttributesNotYetIndexed pick one of the other two.
	attributeOrderingTest  = "ordering_compares_a_numeric_attribute_as_a_number"
	attributeRefusedTest   = "ordering_an_attribute_is_refused_where_it_is_indexed_as_text"
	attributeUnindexedTest = "ordering_an_attribute_finds_nothing_in_indices_written_before_the_numeric_mapping"
	levelRefusedTest       = "a_level_the_backend_does_not_index_is_refused"
	operatorRefusedTest    = "an_operator_the_backend_does_not_evaluate_is_refused"
)

// attributeOrderingTests are the three outcomes of ordering an attribute, of which one runs.
var attributeOrderingTests = []string{attributeOrderingTest, attributeRefusedTest, attributeUnindexedTest}

// Capabilities records what a storage backend *cannot* do in the integration suite. Every
// field is an opt-out: the zero value runs the whole battery, and a backend lists only the
// tests or behaviors it cannot satisfy. New fields must keep that polarity, so a backend
// added later gets full coverage until someone deliberately excuses it from something.
//
// A value is an opt-out claim, exactly the opposite of the opt-in storage capability mechanism of
// ADR-013. An e2e suite may claim more than its direct counterpart, because jaeger-query satisfies
// tests the backend's own reader would fail.
type Capabilities struct {
	// TODO: remove this after all storage backends return spanKind from GetOperations
	getOperationsMissingSpanKind bool
	// TODO: remove this after all storage backends return Source column from GetDependencies
	getDependenciesMissingSource bool
	// searchRequiresServiceName excuses a backend whose reader rejects a search that omits
	// the service name — Cassandra and Badger key every index by it (RFC 0013).
	searchRequiresServiceName bool
	// List of tests which to be skipped (exact name or substring)
	skipList []string
}

// SearchRequiresServiceName returns true if the storage backend cannot serve a search that
// omits the service name.
func (c Capabilities) SearchRequiresServiceName() bool {
	return c.searchRequiresServiceName
}

// GetOperationsMissingSpanKind returns true if the storage backend does not return spanKind from GetOperations.
func (c Capabilities) GetOperationsMissingSpanKind() bool {
	return c.getOperationsMissingSpanKind
}

// GetDependenciesMissingSource returns true if the storage backend does not return the Source column from GetDependencies.
func (c Capabilities) GetDependenciesMissingSource() bool {
	return c.getDependenciesMissingSource
}

// SkipList returns a list of tests that should be skipped for this storage backend.
func (c Capabilities) SkipList() []string {
	return c.skipList
}

// WithoutPagination excuses deployments whose backend does not support continuation tokens.
func (c Capabilities) WithoutPagination() Capabilities {
	c.skipList = append(append([]string(nil), c.skipList...), paginationTest)
	return c
}

// WithoutTraceIDPagination skips trace-ID pagination tests for the e2e adapter,
// which uses Query API v3 and cannot call the storage API's FindTraceIDs method.
func (c Capabilities) WithoutTraceIDPagination() Capabilities {
	c.skipList = append(append([]string(nil), c.skipList...), traceIDPaginationTest)
	return c
}

// WithoutSpanSearch skips span-search assertions for readers that do not implement FindSpans.
func (c Capabilities) WithoutSpanSearch() Capabilities {
	c.skipList = append(append([]string(nil), c.skipList...), spanPaginationTest, spanOrderingTest)
	return c
}

// WithoutSpanSorting excuses readers that cannot execute caller-selected span ordering.
func (c Capabilities) WithoutSpanSorting() Capabilities {
	c.skipList = append(append([]string(nil), c.skipList...), spanOrderingTest)
	return c
}

// WithoutSpanAttributeOrdering skips ordering spans by an attribute, which no backend supports yet.
func (c Capabilities) WithoutSpanAttributeOrdering() Capabilities {
	c.skipList = append(append([]string(nil), c.skipList...), spanAttributeOrderingTest)
	return c
}

// WithoutNumericAttributes declares a deployment that does not configure the typed-attribute
// mapping (RFC 0015), so that ordering an attribute is refused rather than answered. A suite
// that runs with indices.spans.numeric_attributes off uses it.
func (c Capabilities) WithoutNumericAttributes() Capabilities {
	return c.orderingOutcome(attributeRefusedTest)
}

// WithNumericAttributesNotYetIndexed declares a deployment that configures the typed-attribute
// mapping over indices created before it was turned on, so that ordering an attribute is
// evaluated and finds nothing (RFC 0005 §7). The backward-compatibility suite's enable-on-upgrade
// scenario is that deployment.
func (c Capabilities) WithNumericAttributesNotYetIndexed() Capabilities {
	return c.orderingOutcome(attributeUnindexedTest)
}

// orderingOutcome makes the named attribute-ordering case the one that runs, skipping the other
// two whatever the constructor chose.
func (c Capabilities) orderingOutcome(runs string) Capabilities {
	kept := make([]string, 0, len(c.skipList)+2)
	for _, test := range c.skipList {
		if !slices.Contains(attributeOrderingTests, test) {
			kept = append(kept, test)
		}
	}
	for _, test := range attributeOrderingTests {
		if test != runs {
			kept = append(kept, test)
		}
	}
	c.skipList = kept
	return c
}

// WithoutUnindexedLevelRefusal skips the refusal assertion for a filter naming an unindexed level.
// Used for backends that index or evaluate all filter levels (such as memory).
func (c Capabilities) WithoutUnindexedLevelRefusal() Capabilities {
	c.skipList = append(append([]string(nil), c.skipList...), levelRefusedTest)
	return c
}

// WithoutLevelRefusal is an alias for WithoutUnindexedLevelRefusal.
func (c Capabilities) WithoutLevelRefusal() Capabilities {
	return c.WithoutUnindexedLevelRefusal()
}

// WithoutUnevaluatedOperatorRefusal skips the refusal assertion for a filter using an unevaluated operator.
// Used for backends that evaluate all filter operators (such as memory).
func (c Capabilities) WithoutUnevaluatedOperatorRefusal() Capabilities {
	c.skipList = append(append([]string(nil), c.skipList...), operatorRefusedTest)
	return c
}

// WithoutOperatorRefusal is an alias for WithoutUnevaluatedOperatorRefusal.
func (c Capabilities) WithoutOperatorRefusal() Capabilities {
	return c.WithoutUnevaluatedOperatorRefusal()
}

// WithoutTextAttributeOrderingRefusal skips the refusal assertion for numeric ordering on an attribute
// that is indexed as text. Used for backends that compare numeric attributes natively (such as memory)
// or whose indices have typed-attribute mapping enabled.
func (c Capabilities) WithoutTextAttributeOrderingRefusal() Capabilities {
	return c.orderingOutcome(attributeOrderingTest)
}

// WithoutAttributeRefusal is an alias for WithoutTextAttributeOrderingRefusal.
func (c Capabilities) WithoutAttributeRefusal() Capabilities {
	return c.WithoutTextAttributeOrderingRefusal()
}

// WithoutFilterRefusals skips all three refusal assertions in the shared filter battery:
// unindexed level, unevaluated operator, and text-indexed attribute ordering.
// Used by backends that evaluate all of these features natively rather than refusing them.
func (c Capabilities) WithoutFilterRefusals() Capabilities {
	return c.WithoutUnindexedLevelRefusal().
		WithoutUnevaluatedOperatorRefusal().
		WithoutTextAttributeOrderingRefusal()
}

// Memory returns the capabilities for the in-process memory storage backend.
func Memory() Capabilities {
	return Capabilities{
		skipList: []string{
			spanAttributeOrderingTest,
			summaryPaginationTest,
			findTraceSummariesTest,
		},
	}.WithoutFilterRefusals()
}

// GRPC returns the capabilities for the gRPC remote storage backend.
// FindTraceSummaries is skipped because it depends on the backing store computing
// summaries natively; the test backend (memory) does not yet.
func GRPC() Capabilities {
	return Capabilities{
		skipList: []string{
			spanAttributeOrderingTest,
			summaryPaginationTest,
			findTraceSummariesTest,
		},
	}.WithoutFilterRefusals()
}

// Cassandra returns the capabilities for the Cassandra storage backend.
func Cassandra() Capabilities {
	return Capabilities{
		searchRequiresServiceName:    true,
		getDependenciesMissingSource: true,
		skipList: []string{
			spanOrderingTest,
			paginationTest,
			"Tags_+_Operation_name_+_Duration_range",
			"Tags_+_Duration_range",
			"Tags_+_Operation_name_+_max_Duration",
			"Tags_+_max_Duration",
			"Operation_name_+_max_Duration",
			"Multiple_Traces",
			scopeAttributesTest,
			linkAttributesTest,
			findTraceSummariesTest,
			structuredFilterTest,
		},
	}
}

// ClickHouse returns the capabilities for the ClickHouse storage backend.
func ClickHouse() Capabilities {
	return Capabilities{
		skipList: []string{
			spanOrderingTest,
			paginationTest,
			"GetThroughput",
			"GetLatestProbability",
			findTraceSummariesTest,
			structuredFilterTest,
		},
	}
}

// Badger defines the capabilities for the Badger storage backend.
func Badger() Capabilities {
	return Capabilities{
		searchRequiresServiceName: true,
		// TODO: remove this once Badger supports returning spanKind from GetOperations
		getOperationsMissingSpanKind: true,
		skipList: []string{
			spanOrderingTest,
			paginationTest,
			scopeAttributesTest,
			linkAttributesTest,
			findTraceSummariesTest,
			structuredFilterTest,
		},
	}
}

// Elasticsearch defines the capabilities for the Elasticsearch storage backend.
func Elasticsearch() Capabilities {
	return Capabilities{
		// TODO: remove this flag after ES supports returning spanKind
		//  Issue https://github.com/jaegertracing/jaeger/issues/1923
		getOperationsMissingSpanKind: true,
		// The suite configures the typed-attribute mapping (RFC 0015), so an attribute value is
		// indexed as a number beside the keyword and ordering one is answered; orderingOutcome
		// skips the battery's other two ordering outcomes.
		// FindSpans pages and orders by the built-in fields (RFC 0016 M4, M11), not yet by an attribute;
		// the trace searches do not page yet (RFC 0014 M3).
		skipList: []string{
			spanAttributeOrderingTest,
			traceIDPaginationTest,
			summaryPaginationTest,
			scopeAttributesTest,
			linkAttributesTest,
		},
	}.orderingOutcome(attributeOrderingTest)
}

// ElasticsearchSmokeTest defines capabilities for lightweight rotation strategy
// validation tests that skip expensive subtests (large traces, duplicates).
func ElasticsearchSmokeTest() Capabilities {
	return Capabilities{
		getOperationsMissingSpanKind: true,
		skipList: []string{
			spanAttributeOrderingTest,
			scopeAttributesTest,
			linkAttributesTest,
			structuredFilterTest,
			"GetLargeTrace",
			"GetTraceWithDuplicateSpans",
		},
	}
}

// OpenSearch defines the capabilities for the OpenSearch storage backend.
func OpenSearch() Capabilities {
	return Capabilities{
		getOperationsMissingSpanKind: true,
		// Same mapping and same setting, and same search support as Elasticsearch; see the note there.
		skipList: []string{
			spanAttributeOrderingTest,
			traceIDPaginationTest,
			summaryPaginationTest,
			scopeAttributesTest,
			linkAttributesTest,
		},
	}.orderingOutcome(attributeOrderingTest)
}

// Kafka defines the capabilities for the Kafka storage backend.
func Kafka() Capabilities {
	return Capabilities{
		searchRequiresServiceName:    true,
		getDependenciesMissingSource: true,
		skipList: []string{
			spanOrderingTest,
			scopeAttributesTest,
			linkAttributesTest,
			findTraceSummariesTest,
			structuredFilterTest,
		},
	}
}

// E2EWithoutNativeFilters is for an e2e suite whose backend does not evaluate a structured filter
// itself: the query service rewrites one for it, so the battery is all such a suite excuses.
func E2EWithoutNativeFilters() Capabilities {
	return Capabilities{
		skipList: []string{
			spanAttributeOrderingTest,
			structuredFilterTest,
		},
	}
}
