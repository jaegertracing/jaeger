// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
