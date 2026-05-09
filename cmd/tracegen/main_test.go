// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/internal/tracegen"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestRunReturnsTracegenValidationError(t *testing.T) {
	cfg := &tracegen.Config{
		TraceExporter: "stdout",
	}

	err := run(cfg, zap.NewNop())
	require.EqualError(t, err, "either `traces` or `duration` must be greater than 0")
}
