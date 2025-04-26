// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package configschema

import (
	"testing"

	"github.com/crossdock/crossdock-go/assert"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

func TestCollectPackages(t *testing.T) {
	configs := []any{
		&otlpreceiver.Config{},
	}
	packages := collectPackages(configs)
	expected := []string{
		"go.opentelemetry.io/collector/component",
		"go.opentelemetry.io/collector/config/configauth",
		"go.opentelemetry.io/collector/config/configgrpc",
		"go.opentelemetry.io/collector/config/confighttp",
		"go.opentelemetry.io/collector/config/confignet",
		"go.opentelemetry.io/collector/config/configtls",
		"go.opentelemetry.io/collector/receiver/otlpreceiver",
	}
	assert.Equal(t, expected, packages)
}
