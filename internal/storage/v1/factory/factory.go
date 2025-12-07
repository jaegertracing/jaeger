// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

const (
	cassandraStorageType     = "cassandra"
	opensearchStorageType    = "opensearch"
	elasticsearchStorageType = "elasticsearch"
	memoryStorageType        = "memory"
	grpcStorageType          = "grpc"
	badgerStorageType        = "badger"
	blackholeStorageType     = "blackhole"
)

// ArchiveStorage holds archive span reader and writer.
type ArchiveStorage struct {
	Reader spanstore.Reader
	Writer spanstore.Writer
}

// AllStorageTypes defines all available storage backends
var AllStorageTypes = []string{
	cassandraStorageType,
	opensearchStorageType,
	elasticsearchStorageType,
	memoryStorageType,
	badgerStorageType,
	blackholeStorageType,
	grpcStorageType,
}
