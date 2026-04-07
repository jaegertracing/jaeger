// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
)

func TestReader_GetDependencies(t *testing.T) {
	reader := NewDependencyReader()
	ctx := context.Background()
	query := depstore.QueryParameters{}

	dependencies, err := reader.GetDependencies(ctx, query)

	require.Nil(t, dependencies)
	assert.EqualError(t, err, "clickhouse dependency reader is not implemented")
}
