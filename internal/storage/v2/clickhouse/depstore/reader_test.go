// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
)

func TestReader_GetDependencies(t *testing.T) {
	reader := NewDependencyReader()
	ctx := context.Background()
	query := depstore.QueryParameters{}

	require.Panics(t, func() {
		reader.GetDependencies(ctx, query)
	})
}
