// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateOperationsTable_KeyIncludesOperationName(t *testing.T) {
	normalized := strings.Join(strings.Fields(CreateOperationsTable), " ")
	assert.Contains(t, normalized, "ORDER BY (service_name, span_kind, name)")
}

func TestCreateOperationsMaterializedView_DeduplicatesOperations(t *testing.T) {
	normalized := strings.Join(strings.Fields(CreateOperationsMaterializedView), " ")
	assert.Contains(t, normalized, "GROUP BY service_name, name, kind")
}
