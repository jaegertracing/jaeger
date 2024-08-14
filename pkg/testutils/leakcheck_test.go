// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package testutils_test

import (
	"testing"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestVerifyGoLeaksOnce(t *testing.T) {
	defer testutils.VerifyGoLeaksOnce(t)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
