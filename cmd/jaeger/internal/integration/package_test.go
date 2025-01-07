// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestMain(m *testing.M) {
	// The tests of elastic search include tests of ES-Rollover which run tests from integration of v1
	// Those tests use the official es-clients which are responsible for the leakages
	if os.Getenv("STORAGE") == "elasticsearch" {
		testutils.VerifyGoLeaksForES(m)
	} else {
		testutils.VerifyGoLeaks(m)
	}
}
