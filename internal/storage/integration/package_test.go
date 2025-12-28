// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func runTestMain(m *testing.M, storage string) {
	if storage == "elasticsearch" || storage == "opensearch" {
		testutils.VerifyGoLeaksForES(m)
	} else {
		testutils.VerifyGoLeaks(m)
	}
}

func TestMain(m *testing.M) {
	runTestMain(m, os.Getenv("STORAGE"))
}
