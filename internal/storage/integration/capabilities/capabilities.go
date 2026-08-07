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
	FindTraceSummariesTest = "FindTraceSummaries"
)

// Capabilities defines the capabilities of a storage backend for integration tests.
type Capabilities struct {
	// TODO: remove this after all storage backends return spanKind from GetOperations
	getOperationsMissingSpanKind bool
	// TODO: remove this after all storage backends return Source column from GetDependencies
	getDependenciesMissingSource bool
	// List of tests which to be skipped (exact name or substring)
	skipList []string
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
		skipList: []string{FindTraceSummariesTest},
	}
}

// GRPC returns the capabilities for the gRPC remote storage backend.
// FindTraceSummaries is skipped because it depends on the backing store computing
// summaries natively; the test backend (memory) does not yet.
func GRPC() Capabilities {
	return Capabilities{
		skipList: []string{FindTraceSummariesTest},
	}
}

// Cassandra returns the capabilities for the Cassandra storage backend.
func Cassandra() Capabilities {
	return Capabilities{
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
		},
	}
}

// ClickHouse returns the capabilities for the ClickHouse storage backend.
func ClickHouse() Capabilities {
	return Capabilities{
		skipList: []string{"GetThroughput", "GetLatestProbability", FindTraceSummariesTest},
	}
}

// Badger defines the capabilities for the Badger storage backend.
func Badger() Capabilities {
	return Capabilities{
		// TODO: remove this once Badger supports returning spanKind from GetOperations
		getOperationsMissingSpanKind: true,
		skipList:                     []string{scopeAttributesTest, linkAttributesTest, FindTraceSummariesTest},
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
		getDependenciesMissingSource: true,
		skipList:                     []string{scopeAttributesTest, linkAttributesTest, FindTraceSummariesTest},
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
