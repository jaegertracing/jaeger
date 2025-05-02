// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	if os.Getenv("STORAGE") == "elasticsearch" || os.Getenv("STORAGE") == "opensearch" {
		testutils.VerifyGoLeaksForES(m)
	} else {
		testutils.VerifyGoLeaks(m)
	}
}
