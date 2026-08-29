// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

import (
	"os"
	"strconv"
)

const (
	scopeAttributesTest    = "Scope_Attributes"
	linkAttributesTest     = "Link_Attributes"
	findTraceSummariesTest = "FindTraceSummaries"
	structuredFilterTest   = "FindTracesWithFilter"
	attributeOrderingTest  = "ordering_compares_a_numeric_attribute_as_a_number"
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
	if IsBackwardCompatibilityEnv() {
		// TODO: Write a separate backward-compatibility tests for testing GetServices
		c.skipList = appendIfNotExists(c.skipList, "GetServices")
		// This test is skipped because it provides no value in testing backward
		// compatibility and makes the whole test very heavy due to large number
		// of traces being injected to jaeger
		c.skipList = appendIfNotExists(c.skipList, "GetLargeTrace")
		// This test is not applicable to backward-compatibility
		c.skipList = appendIfNotExists(c.skipList, "NotFound_error")
		// TODO: Support FindTraceSummaries test for backward compatibility testing
		// Probably by adding a new fixture and adding more constraints to the query
		c.skipList = appendIfNotExists(c.skipList, "FindTraceSummaries")
	}
	return c.skipList
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
		// This schema indexes every attribute as a keyword, so ordering one is refused rather than
		// answered lexicographically, and the battery's paired refusal case is the one to run
		// (RFC 0015 is what changes that).
		skipList: []string{scopeAttributesTest, linkAttributesTest, attributeOrderingTest},
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
		// Same keyword mapping as Elasticsearch; see the note there.
		skipList: []string{scopeAttributesTest, linkAttributesTest, attributeOrderingTest},
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

func IsBackwardCompatibilityEnv() bool {
	is, _ := strconv.ParseBool(os.Getenv("BACKWARD_COMPATIBILITY"))
	return is
}

func appendIfNotExists(list []string, item string) []string {
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	return append(list, item)
}
