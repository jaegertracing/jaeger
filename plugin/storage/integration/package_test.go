// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	goleakTestutils "github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestMain(m *testing.M) {
	goleakTestutils.VerifyGoLeaks(m)
}
