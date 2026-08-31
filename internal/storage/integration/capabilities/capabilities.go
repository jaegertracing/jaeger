// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

const (
	scopeAttributesTest    = "Scope_Attributes"
	linkAttributesTest     = "Link_Attributes"
	findTraceSummariesTest = "FindTraceSummaries"
	structuredFilterTest   = "FindTracesWithFilter"

	// The battery pairs these two: ordering an attribute is answered where the index carries the
	// typed-attribute mapping (RFC 0015) and refused where it does not, so exactly one of them runs
	// for any deployment. Elasticsearch and OpenSearch skip the refusal, because the suites that
	// run the battery enable the mapping; WithoutTypedAttributeIndexing swaps them back.
	attributeOrderingTest = "ordering_compares_a_numeric_attribute_as_a_number"
	attributeRefusedTest  = "ordering_an_attribute_is_refused_where_it_is_indexed_as_text"
)

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

// WithoutTypedAttributeIndexing declares a deployment reading indices that were created without the
// typed-attribute mapping (RFC 0015), so that ordering an attribute is refused rather than answered.
// It swaps which of the battery's two paired ordering cases runs.
//
// An index template reaches only indices created after it is installed, so this is not the same
// question as whether the binary has the feature gate on. A read phase whose gate is on, over an
// index a previous phase created with the gate off, still has no numeric sub-field to range over —
// which is why the backward-compatibility suites use this and the ordinary e2e suites do not.
func (c Capabilities) WithoutTypedAttributeIndexing() Capabilities {
	swapped := make([]string, 0, len(c.skipList)+1)
	for _, test := range c.skipList {
		if test != attributeRefusedTest {
			swapped = append(swapped, test)
		}
	}
	c.skipList = append(swapped, attributeOrderingTest)
	return c
}

// Memory returns the capabilities for the in-process memory storage backend.
func Memory() Capabilities {
	return Capabilities{
		skipList: []string{findTraceSummariesTest, structuredFilterTest},
	}
}

// GRPC returns the capabilities for the gRPC remote storage backend.
// FindTraceSummaries is skipped because it depends on the backing store computing
// summaries natively; the test backend (memory) does not yet.
func GRPC() Capabilities {
	return Capabilities{
		skipList: []string{findTraceSummariesTest, structuredFilterTest},
	}
}

// Cassandra returns the capabilities for the Cassandra storage backend.
func Cassandra() Capabilities {
	return Capabilities{
		searchRequiresServiceName:    true,
		getDependenciesMissingSource: true,
		skipList: []string{
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
		skipList: []string{"GetThroughput", "GetLatestProbability", findTraceSummariesTest, structuredFilterTest},
	}
}

// Badger defines the capabilities for the Badger storage backend.
func Badger() Capabilities {
	return Capabilities{
		searchRequiresServiceName: true,
		// TODO: remove this once Badger supports returning spanKind from GetOperations
		getOperationsMissingSpanKind: true,
		skipList:                     []string{scopeAttributesTest, linkAttributesTest, findTraceSummariesTest, structuredFilterTest},
	}
}

// Elasticsearch defines the capabilities for the Elasticsearch storage backend.
func Elasticsearch() Capabilities {
	return Capabilities{
		// TODO: remove this flag after ES supports returning spanKind
		//  Issue https://github.com/jaegertracing/jaeger/issues/1923
		getOperationsMissingSpanKind: true,
		// The suite runs with typed attribute indexing enabled (RFC 0015), so an attribute value is
		// indexed as a number beside the keyword and ordering one is answered rather than refused.
		// That makes the battery's paired refusal case the one to skip.
		skipList: []string{scopeAttributesTest, linkAttributesTest, attributeRefusedTest},
	}
}

// ElasticsearchSmokeTest defines capabilities for lightweight rotation strategy
// validation tests that skip expensive subtests (large traces, duplicates).
func ElasticsearchSmokeTest() Capabilities {
	return Capabilities{
		getOperationsMissingSpanKind: true,
		skipList: []string{
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
		// Same mapping and same gate as Elasticsearch; see the note there.
		skipList: []string{scopeAttributesTest, linkAttributesTest, attributeRefusedTest},
	}
}

// Kafka defines the capabilities for the Kafka storage backend.
func Kafka() Capabilities {
	return Capabilities{
		searchRequiresServiceName:    true,
		getDependenciesMissingSource: true,
		skipList:                     []string{scopeAttributesTest, linkAttributesTest, findTraceSummariesTest, structuredFilterTest},
	}
}

// E2EWithoutNativeFilters is for an e2e suite whose backend does not evaluate a structured filter
// itself: the query service rewrites one for it, so the battery is all such a suite excuses.
func E2EWithoutNativeFilters() Capabilities {
	return Capabilities{
		skipList: []string{structuredFilterTest},
	}
}
