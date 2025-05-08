// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client/mocks"
)

func TestNewTraceWriter(t *testing.T) {
	conn := new(mocks.Conn)
	writer := NewTraceWriter(conn)
	require.NotNil(t, writer)
}
