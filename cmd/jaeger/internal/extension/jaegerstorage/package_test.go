// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"testing"

	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	if err := featuregate.GlobalRegistry().Set("storage.clickhouse", true); err != nil {
		panic(err)
	}
	testutils.VerifyGoLeaks(m)
}
