// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestMain(m *testing.M) {
	switch os.Getenv("STORAGE") {
	case "elasticsearch":
	case "opensearch":
		testutils.VerifyGoLeaksForES(m)
	case "clickhouse":
		testutils.VerifyGoLeaksForClickhouse(m)
	default:
		testutils.VerifyGoLeaks(m)
	}
}
