// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package setupcontext

import (
	"testing"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestSetupContext(_ *testing.T) {
	// purely for code coverage
	SetAllInOne()
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
