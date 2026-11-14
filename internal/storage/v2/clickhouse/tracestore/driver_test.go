// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
)

func verifyQuerySnapshot(t *testing.T, queries ...string) {
	t.Helper()
	clickhousetest.VerifyQuerySnapshot(t, queries...)
}
