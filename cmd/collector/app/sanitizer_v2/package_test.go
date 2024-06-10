// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer_v2

import (
	"testing"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
