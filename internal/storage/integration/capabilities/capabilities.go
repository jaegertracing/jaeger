// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

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

	// attributeComparisonTest asserts the outcome the deployment declares through
	// AttributeComparison; a backend that evaluates no ordered comparison on an attribute skips it.
	attributeComparisonTest = "an_ordered_comparison_on_an_attribute"

	// Filter cases on built-in fields that a backend's lowering does not map, which the levels and
	// operators of its FilterCapabilities are too coarse to say. A case on a level or an operator
	// needs no entry here: the battery reads the declaration and expects a refusal instead.
	filterEventNameTest  = "the_name_of_one_of_the_span's_events"
	filterSpanKindTest   = "the_span_kind"
	filterSpanStatusTest = "the_span_status"
)

// AttributeComparison is the outcome a deployment produces for an ordered comparison on an
// attribute, such as `span.retry.count > 10` (RFC 0005 §7). The operator and the level are both
// declared, so the capability check admits the predicate, and the index mapping decides whether
// the value is stored as a number. The zero value is the fully capable outcome, keeping the
// polarity of Capabilities.
//
// Elasticsearch and OpenSearch store a numeric sub-field beside the keyword only when
// indices.spans.numeric_attributes is on (RFC 0015), and only in indices created after it was
// turned on. A deployment is therefore in one of three states: the setting is on and the data
// was written under it, the setting is off, or the setting was turned on at an upgrade and the
// reader ranges over the numeric sub-field in indices that never got one. The backward
// compatibility suites run the last two against an old binary that wrote the corpus.
type AttributeComparison int

const (
	// AttributeComparisonNumeric declares that the value is indexed as a number, so the
	// comparison is answered numerically.
	AttributeComparisonNumeric AttributeComparison = iota
	// AttributeComparisonRefused declares that the value is indexed as text, so the reader
	// refuses the comparison rather than ranging over the keyword.
	AttributeComparisonRefused
	// AttributeComparisonMissesOlderIndices declares that the mapping was turned on after the
	// data was written, so the comparison is evaluated and finds nothing in those indices.
	AttributeComparisonMissesOlderIndices
)

// Capabilities records what a storage backend *cannot* do in the integration suite. Every field
// is an opt-out: the zero value runs the whole battery, and a backend lists only the tests it
// cannot pass. New fields must keep that polarity, so a backend added later gets full coverage
// until someone deliberately excuses it from something. This is the opposite of the opt-in
// storage capability mechanism of ADR-013, where a reader names what it can do. An e2e suite may
// claim more than its direct counterpart, because jaeger-query satisfies tests the backend's own
// reader would fail.
//
// No double negatives. An opt-out lets a backend skip a test that checks whether it can do
// something. It must never let a backend skip a test that checks whether it refuses something,
// because the backends that evaluate the predicate instead of refusing it would then carry an
// opt-out from a negative assertion, which a reader would misread as a capability they lack. A
// refusal test therefore takes its expected outcome from what the reader declares in
// FilterCapabilities, or from a typed field on this struct that names the outcome
// (AttributeComparison, traceStateRefused), and asserts that declared outcome on every backend.
// The WithoutFilterRefusals opt-outs removed in https://github.com/jaegertracing/jaeger/pull/9807
// are an example of the pattern this rule forbids.
type Capabilities struct {
	// TODO: remove this after all storage backends return spanKind from GetOperations
	getOperationsMissingSpanKind bool
	// TODO: remove this after all storage backends return Source column from GetDependencies
	getDependenciesMissingSource bool
	// searchRequiresServiceName excuses a backend whose reader rejects a search that omits
	// the service name — Cassandra and Badger key every index by it (RFC 0013).
	searchRequiresServiceName bool
	// pagingDropsTiedSpans excuses a reader whose span search, at its default configuration,
	// loses the later occurrences of documents that tie on every sort key when they straddle a
	// page boundary (RFC 0016 §6.4): Elasticsearch and OpenSearch sort on _id only when
	// span_search_tie_break_by_id is on.
	pagingDropsTiedSpans bool
	// attributeComparison is the outcome the filter battery expects for an ordered comparison on
	// an attribute; the zero value expects the comparison answered numerically.
	attributeComparison AttributeComparison
	// traceStateRefused excuses a reader that does not index span.traceState and refuses a
	// filter naming it (RFC 0005 §7). FilterCapabilities declares levels and operators, not
	// built-in fields, so the deployment declares this one here.
	traceStateRefused bool
	// List of tests which to be skipped (exact name or substring)
	skipList []string
}

// AttributeComparison returns the outcome the filter battery expects for an ordered comparison on
// an attribute.
func (c Capabilities) AttributeComparison() AttributeComparison {
	return c.attributeComparison
}

// TraceStateRefused returns true if the reader refuses a filter on span.traceState rather than
// evaluating it.
func (c Capabilities) TraceStateRefused() bool {
	return c.traceStateRefused
}

// PagingDropsTiedSpans returns true if a span search may skip documents that tie on every sort
// key across a page boundary.
func (c Capabilities) PagingDropsTiedSpans() bool {
	return c.pagingDropsTiedSpans
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
// mapping (RFC 0015), so that an ordered comparison on an attribute is refused rather than answered. A suite
// that runs with indices.spans.numeric_attributes off uses it.
func (c Capabilities) WithoutNumericAttributes() Capabilities {
	c.attributeComparison = AttributeComparisonRefused
	return c
}

// WithNumericAttributesEnabledAtUpgrade declares a deployment that turned the typed-attribute
// mapping on at an upgrade, over indices written without it, so that an ordered comparison on an
// attribute is evaluated and finds nothing (RFC 0005 §7).
func (c Capabilities) WithNumericAttributesEnabledAtUpgrade() Capabilities {
	c.attributeComparison = AttributeComparisonMissesOlderIndices
	return c
}

// Memory returns the capabilities for the in-process memory storage backend.
func Memory() Capabilities {
	return Capabilities{
		skipList: []string{
			spanAttributeOrderingTest,
			// Trace summaries not supported; jaeger-query falls back to FindTraces for the e2e suite.
			summaryPaginationTest,
			findTraceSummariesTest,
		},
	}
}

// GRPC returns the capabilities for the gRPC remote storage backend, whose server runs the memory
// store, so the list is the memory store's.
func GRPC() Capabilities {
	return Memory()
}

// Cassandra returns the capabilities for the Cassandra storage backend.
func Cassandra() Capabilities {
	return Capabilities{
		searchRequiresServiceName:    true,
		getDependenciesMissingSource: true,
		skipList: []string{
			// The reader implements neither FindSpans nor continuation tokens.
			spanOrderingTest,
			paginationTest,
			// A search with a duration bound reads the duration_index table alone, which ignores
			// the tag and operation predicates sent with it (ADR-001).
			"Tags_+_Operation_name_+_Duration_range",
			"Tags_+_Duration_range",
			"Tags_+_Operation_name_+_max_Duration",
			"Tags_+_max_Duration",
			"Operation_name_+_max_Duration",
			// The reader goes through the v1 span model, which has no scope or link attributes.
			scopeAttributesTest,
			linkAttributesTest,
			// Trace summaries not supported; jaeger-query falls back to FindTraces for the e2e suite.
			findTraceSummariesTest,
			// The reader declares no filter levels or operators.
			structuredFilterTest,
		},
	}
}

// clickHouseSkipList is what the ClickHouse reader does not satisfy in either mode. It returns a
// fresh slice, so a caller may append to it.
func clickHouseSkipList() []string {
	return []string{
		// The reader implements neither FindSpans nor continuation tokens.
		spanOrderingTest,
		paginationTest,
		// The lowering maps five built-in fields; event.name has a column but is not among them yet.
		filterEventNameTest,
		// The lowering declares the ordered comparisons but evaluates them on span.duration only, and
		// refuses one on an attribute inside the lowering (RFC 0005 M3, first increment).
		attributeComparisonTest,
	}
}

// ClickHouse returns the capabilities for the ClickHouse storage backend read directly.
func ClickHouse() Capabilities {
	return Capabilities{
		// The lowering has no mapping for span.traceState yet and refuses a filter naming it.
		traceStateRefused: true,
		skipList: append(
			clickHouseSkipList(),
			// Trace summaries not supported; jaeger-query falls back to FindTraces for the e2e suite.
			findTraceSummariesTest,
			// The direct suite has no sampling store for ClickHouse.
			"GetThroughput",
			"GetLatestProbability",
		),
	}
}

// ClickHouseE2E returns the capabilities for the ClickHouse e2e suite, which reaches the reader
// through jaeger-query.
func ClickHouseE2E() Capabilities {
	return Capabilities{
		traceStateRefused: true,
		skipList:          clickHouseSkipList(),
	}
}

// Badger defines the capabilities for the Badger storage backend.
func Badger() Capabilities {
	return Capabilities{
		searchRequiresServiceName: true,
		// TODO: remove this once Badger supports returning spanKind from GetOperations
		getOperationsMissingSpanKind: true,
		skipList: []string{
			// The reader implements neither FindSpans nor continuation tokens.
			spanOrderingTest,
			paginationTest,
			// The reader goes through the v1 span model, which has no scope or link attributes.
			scopeAttributesTest,
			linkAttributesTest,
			// Trace summaries not supported; jaeger-query falls back to FindTraces for the e2e suite.
			findTraceSummariesTest,
			// The reader declares no filter levels or operators.
			structuredFilterTest,
		},
	}
}

// Elasticsearch defines the capabilities for the Elasticsearch storage backend. OpenSearch shares
// its mapping and its lowering, so OpenSearch and ElasticsearchSmokeTest derive from it.
func Elasticsearch() Capabilities {
	return Capabilities{
		// TODO: remove this flag after ES supports returning spanKind
		//  Issue https://github.com/jaegertracing/jaeger/issues/1923
		getOperationsMissingSpanKind: true,
		// The direct-mode suite runs this set against OpenSearch as well, with the reader's
		// default configuration, so the drop applies there too.
		pagingDropsTiedSpans: true,
		// The span document has no field for the trace state, so a filter naming it is refused.
		traceStateRefused: true,
		skipList: []string{
			// FindSpans orders by the built-in fields (RFC 0016 M4, M11), not yet by an attribute.
			spanAttributeOrderingTest,
			// The trace searches do not page yet (RFC 0014 M3).
			traceIDPaginationTest,
			summaryPaginationTest,
			// The span document folds the scope's attributes into the span's tags and keeps a link
			// as a reference without its attributes, so neither comes back as written.
			scopeAttributesTest,
			linkAttributesTest,
			// The span document stores the kind and the status as tags, which the lowering does
			// not map; the levels and operators of its FilterCapabilities are too coarse to say so.
			filterSpanKindTest,
			filterSpanStatusTest,
		},
	}
}

// ElasticsearchSmokeTest defines capabilities for the rotation strategy suites, which check that a
// write lands in the rotated index and a read finds it there, and so leave out the filter battery
// and the two slowest subtests.
func ElasticsearchSmokeTest() Capabilities {
	c := Elasticsearch()
	c.skipList = append(c.skipList,
		structuredFilterTest,
		"GetLargeTrace",
		"GetTraceWithDuplicateSpans",
	)
	return c
}

// OpenSearch defines the capabilities for the OpenSearch e2e suite, whose configuration turns
// span_search_tie_break_by_id on, so paging keeps every tied occurrence.
func OpenSearch() Capabilities {
	c := Elasticsearch()
	c.pagingDropsTiedSpans = false
	return c
}

// Kafka defines the capabilities for the Kafka storage backend.
func Kafka() Capabilities {
	return Capabilities{
		searchRequiresServiceName:    true,
		getDependenciesMissingSource: true,
		skipList: []string{
			spanAttributeOrderingTest,
			// The jaeger_proto and jaeger_json encodings go through the v1 span model, which keeps
			// no scope or link attributes, and the suite runs one list for all four encodings.
			scopeAttributesTest,
			linkAttributesTest,
			// The ingester's configuration does not turn on the structured-filter feature gate of
			// jaeger-query.
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
			// jaeger-query rewrites a filter into the legacy predicate fields for such a backend,
			// which the battery's cases cannot be expressed in.
			structuredFilterTest,
		},
	}
}
