// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestLoggingRotation_NoDebug(t *testing.T) {
	inner := NewPeriodicRotation("jaeger-span", "2006-01-02", 24*time.Hour)
	r := NewLoggingRotation(inner, zap.NewNop())
	assert.Equal(t, inner, r)
}

func TestLoggingRotation_WithDebug(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))
	inner := NewPeriodicRotation("jaeger-span", "2006-01-02", 24*time.Hour)
	r := NewLoggingRotation(inner, logger)
	assert.IsType(t, &LoggingRotation{}, r)

	date := time.Date(1995, time.April, 21, 4, 0, 0, 0, time.UTC)
	assert.Equal(t, "jaeger-span-1995-04-21", r.WriteTarget(date))
	assert.Equal(t, []string{"jaeger-span-1995-04-21"}, r.ReadTargets(date, date))
	assert.Equal(t, WriteOpIndex, r.WriteOpType())
}
