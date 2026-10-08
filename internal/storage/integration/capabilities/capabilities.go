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
	levelRefusedTest        = "a_level_the_backend_does_not_index_is_refused"
	operatorRefusedTest     = "an_operator_the_backend_does_not_evaluate_is_refused"

	// Filter cases using operators, fields, or levels a backend does not evaluate natively.
	filterUnqualifiedEventTest = "an_unqualified_attribute_does_not_reach_the_event_level"
	filterServiceInListTest    = "the_service_name_against_a_list_of_names"
	filterOperationRegexTest   = "a_pattern_on_the_operation_name_matches_anywhere_in_it"
	filterEventNameTest        = "the_name_of_one_of_the_span's_events"
	filterDurationGtTest       = "a_duration_greater_than_a_bound"
	filterDurationLteTest      = "a_duration_at_most_a_bound"
	filterDurationRangeTest    = "a_duration_between_two_bounds"
	filterAttributeNeTest      = "an_attribute_inequality_leaves_out_a_span_that_lacks_the_attribute"
	filterAttributeExistsTest  = "an_attribute_exists"
	filterAttributeRegexTest   = "a_pattern_on_an_attribute_value"
	filterFieldDurationAndTest = "a_conjunction_of_a_built-in_field_and_a_duration"
	filterScopeLevelTest       = "a_scope-level_attribute_matches_the_instrumentation_scope's_attributes_only"
	filterLinkLevelTest        = "a_link-level_attribute_matches_an_attribute_of_one_of_the_span's_links"
	filterSpanKindTest         = "the_span_kind"
	filterSpanStatusTest       = "the_span_status"
	filterStringTypedConstTest = "a_string-typed_constant_leaves_out_an_attribute_stored_as_a_number"
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

// filterOperatorTests are the battery cases that need ne, regex, exists or in, or an ordered
// comparison on an attribute. A backend whose lowering evaluates equality on attributes and
// compares only span.duration skips them as a set.
var filterOperatorTests = []string{
	filterUnqualifiedEventTest,
	filterServiceInListTest,
	filterOperationRegexTest,
	filterAttributeNeTest,
	filterAttributeExistsTest,
	filterAttributeRegexTest,
	attributeComparisonTest,
}

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

// WithoutFilterRefusals skips both refusal assertions in the shared filter battery, the unindexed
// level and the unevaluated operator. Used by backends that evaluate every level and operator
// natively rather than refusing any.
func (c Capabilities) WithoutFilterRefusals() Capabilities {
	return c.WithoutUnindexedLevelRefusal().
		WithoutUnevaluatedOperatorRefusal()
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

// clickHouseSkipList is what the ClickHouse reader does not satisfy in either mode.
var clickHouseSkipList = append([]string{
	spanOrderingTest,
	paginationTest,
	// The lowering maps five built-in fields; event.name has a column but is not among them yet.
	filterEventNameTest,
	// The reader indexes all five levels, so there is no level to refuse.
	levelRefusedTest,
	// The lowering evaluates and, or, not and eq, and compares only span.duration (RFC 0005 M3,
	// first increment).
}, filterOperatorTests...)

// ClickHouse returns the capabilities for the ClickHouse storage backend read directly.
func ClickHouse() Capabilities {
	return Capabilities{
		// The lowering has no mapping for span.traceState yet and refuses a filter naming it.
		traceStateRefused: true,
		skipList: append([]string{
			// The ClickHouse reader does not support FindTraceSummaries. They are tested in
			// the e2e suite because the query service falls back to FindTraces.
			findTraceSummariesTest,
			// The direct suite has no sampling store for ClickHouse.
			"GetThroughput",
			"GetLatestProbability",
		}, clickHouseSkipList...),
	}
}

// ClickHouseE2E returns the capabilities for the ClickHouse e2e suite, which reaches the reader
// through jaeger-query.
func ClickHouseE2E() Capabilities {
	return Capabilities{
		traceStateRefused: true,
		skipList:          clickHouseSkipList,
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
			scopeAttributesTest,
			linkAttributesTest,
			filterScopeLevelTest,
			filterLinkLevelTest,
			filterSpanKindTest,
			filterSpanStatusTest,
		},
	}
}

// ElasticsearchSmokeTest defines capabilities for lightweight rotation strategy
// validation tests that skip expensive subtests (large traces, duplicates).
func ElasticsearchSmokeTest() Capabilities {
	return Capabilities{
		getOperationsMissingSpanKind: true,
		// The rotation configurations, for OpenSearch as well as Elasticsearch, leave
		// span_search_tie_break_by_id at its default.
		pagingDropsTiedSpans: true,
		traceStateRefused:    true,
		skipList: []string{
			spanAttributeOrderingTest,
			traceIDPaginationTest,
			summaryPaginationTest,
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
		traceStateRefused:            true,
		// Same mapping and same setting, and same search support as Elasticsearch; see the note there.
		// The e2e configuration turns span_search_tie_break_by_id on, so paging keeps every tied
		// occurrence and pagingDropsTiedSpans stays unset.
		skipList: []string{
			spanAttributeOrderingTest,
			traceIDPaginationTest,
			summaryPaginationTest,
			scopeAttributesTest,
			linkAttributesTest,
			filterScopeLevelTest,
			filterLinkLevelTest,
			filterSpanKindTest,
			filterSpanStatusTest,
		},
	}
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
