// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package thrift

import (
	"testing"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestDummy(*testing.T) {
	// This is a dummy test in the root package.
	// Without it `go test -v .` prints "testing: warning: no tests to run".
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
