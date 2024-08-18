// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package setupcontext

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestSetupContext(t *testing.T) {
	// purely for code coverage
	SetAllInOne()
	defer UnsetAllInOne()
	assert.True(t, IsAllInOne())
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
