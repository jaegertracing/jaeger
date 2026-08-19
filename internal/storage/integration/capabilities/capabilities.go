// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

const (
	scopeAttributesTest    = "Scope_Attributes"
	linkAttributesTest     = "Link_Attributes"
	FindTraceSummariesTest = "FindTraceSummaries"
	// structuredFilterTest names the RFC 0005 filter battery, which asserts exact result sets and
	// so runs only where the reader evaluates a filter itself. A reader that declares no filter
	// capability is handed the filter rewritten into the legacy predicate fields instead, which
	// drop the level a predicate named and answer with a superset (RFC 0005 §7) — and the battery
	// is written to catch exactly that. Each backend drops this as it gains native filter support.
	structuredFilterTest = "FindTracesWithFilter"
)

// Capabilities records what a storage backend *cannot* do in the integration suite. Every
// field is an opt-out: the zero value runs the whole battery, and a backend lists only the
// tests or behaviors it cannot satisfy. New fields must keep that polarity, so a backend
// added later gets full coverage until someone deliberately excuses it from something.
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

// Memory returns the capabilities for the in-process memory storage backend.
func Memory() Capabilities {
	return Capabilities{
		skipList: []string{FindTraceSummariesTest, structuredFilterTest},
	}
}

// GRPC returns the capabilities for the gRPC remote storage backend.
// FindTraceSummaries is skipped because it depends on the backing store computing
// summaries natively; the test backend (memory) does not yet.
func GRPC() Capabilities {
	return Capabilities{
		skipList: []string{FindTraceSummariesTest, structuredFilterTest},
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
			FindTraceSummariesTest,
			structuredFilterTest,
		},
	}
}

// ClickHouse returns the capabilities for the ClickHouse storage backend.
func ClickHouse() Capabilities {
	return Capabilities{
		skipList: []string{"GetThroughput", "GetLatestProbability", FindTraceSummariesTest, structuredFilterTest},
	}
}

// Badger defines the capabilities for the Badger storage backend.
func Badger() Capabilities {
	return Capabilities{
		searchRequiresServiceName: true,
		// TODO: remove this once Badger supports returning spanKind from GetOperations
		getOperationsMissingSpanKind: true,
		skipList:                     []string{scopeAttributesTest, linkAttributesTest, FindTraceSummariesTest, structuredFilterTest},
	}
}

// Elasticsearch defines the capabilities for the Elasticsearch storage backend.
func Elasticsearch() Capabilities {
	return Capabilities{
		// TODO: remove this flag after ES supports returning spanKind
		//  Issue https://github.com/jaegertracing/jaeger/issues/1923
		getOperationsMissingSpanKind: true,
		skipList:                     []string{scopeAttributesTest, linkAttributesTest},
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
		skipList:                     []string{scopeAttributesTest, linkAttributesTest},
	}
}

// Kafka defines the capabilities for the Kafka storage backend.
func Kafka() Capabilities {
	return Capabilities{
		searchRequiresServiceName:    true,
		getDependenciesMissingSource: true,
		skipList:                     []string{scopeAttributesTest, linkAttributesTest, FindTraceSummariesTest, structuredFilterTest},
	}
}

// NoStructuredFilters excuses a suite from the RFC 0005 filter battery and nothing else, for a
// backend that satisfies the rest of the suite but whose reader does not evaluate a structured
// filter. It is what the e2e suites need, because a backend reached through jaeger-query can
// satisfy tests its own reader cannot — computing trace summaries, for one — so those suites
// cannot reuse the per-backend capabilities their direct counterparts declare.
func NoStructuredFilters() Capabilities {
	return Capabilities{
		skipList: []string{structuredFilterTest},
	}
}
