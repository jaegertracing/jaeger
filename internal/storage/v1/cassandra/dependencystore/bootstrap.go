// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2019 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"github.com/jaegertracing/jaeger/internal/storage/cassandra"
)

// GetDependencyVersion attempts to determine the version of the dependencies table.
// TODO: Remove this once we've migrated to V2 permanently. https://github.com/jaegertracing/jaeger/issues/1344
func GetDependencyVersion(s cassandra.Session) Version {
	if err := s.Query("SELECT ts from dependencies_v2 limit 1;").Exec(); err != nil {
		return V1
	}
	return V2
}
