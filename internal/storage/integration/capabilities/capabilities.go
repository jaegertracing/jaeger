// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package capabilities

// Capabilities defines the capabilities of a storage backend for integration tests.
type Capabilities struct {
	getOperationsMissingSpanKind bool
	getDependenciesReturnsSource bool
	skipList                     []string
}

// GetOperationsMissingSpanKind returns true if the storage backend does not return spanKind from GetOperations.
func (c Capabilities) GetOperationsMissingSpanKind() bool {
	return c.getOperationsMissingSpanKind
}

// GetDependenciesReturnsSource returns true if the storage backend returns the Source column from GetDependencies.
func (c Capabilities) GetDependenciesReturnsSource() bool {
	return c.getDependenciesReturnsSource
}

// SkipList returns a list of tests that should be skipped for this storage backend.
func (c Capabilities) SkipList() []string {
	return c.skipList
}

// Cassandra returns the capabilities for the Cassandra storage backend.
func Cassandra() Capabilities {
	c := Capabilities{}
	c.getDependenciesReturnsSource = true
	c.skipList = []string{
		"Tags_+_Operation_name_+_Duration_range",
		"Tags_+_Duration_range",
		"Tags_+_Operation_name_+_max_Duration",
		"Tags_+_max_Duration",
		"Operation_name_+_max_Duration",
		"Multiple_Traces",
	}
	return c
}

// ClickHouse returns the capabilities for the ClickHouse storage backend.
func ClickHouse() Capabilities {
	c := Capabilities{}
	c.skipList = []string{"GetThroughput", "GetLatestProbability"}
	return c
}

// Badger defines the capabilities for the Badger storage backend.
func Badger() Capabilities {
	c := Capabilities{}
	// TODO: remove this badger supports returning spanKind from GetOperations
	c.getOperationsMissingSpanKind = true
	return c
}

// Elasticsearch defines the capabilities for the Elasticsearch storage backend.
func Elasticsearch() Capabilities {
	c := Capabilities{}
	// TODO: remove this flag after ES supports returning spanKind
	//  Issue https://github.com/jaegertracing/jaeger/issues/1923
	c.getOperationsMissingSpanKind = true
	return c
}

// OpenSearch defines the capabilities for the OpenSearch storage backend.
func OpenSearch() Capabilities {
	c := Capabilities{}
	c.getOperationsMissingSpanKind = true
	return c
}

// Kafka defines the capabilities for the Kafka storage backend.
func Kafka() Capabilities {
	c := Capabilities{}
	c.getDependenciesReturnsSource = true
	return c
}
