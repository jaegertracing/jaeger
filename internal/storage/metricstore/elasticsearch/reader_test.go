// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestPlaceholder(_ *testing.T) {
	// Empty for later
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
