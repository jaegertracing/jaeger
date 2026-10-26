// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, "http://localhost:9090", cfg.ServerURL)
	assert.Equal(t, 30*time.Second, cfg.ConnectTimeout)
	assert.Equal(t, "traces_span_metrics", cfg.MetricNamespace)
	assert.Equal(t, "ms", cfg.LatencyUnit)
	assert.False(t, cfg.NormalizeCalls)
	assert.False(t, cfg.NormalizeDuration)
}
