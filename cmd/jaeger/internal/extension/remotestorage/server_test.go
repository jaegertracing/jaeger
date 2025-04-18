// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotestorage

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
)

func TestServerDependencies(t *testing.T) {
	expectedDependencies := []component.ID{jaegerstorage.ID}
	telemetrySettings := component.TelemetrySettings{
		Logger: zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller())),
	}

	server := newServer(createDefaultConfig().(*Config), telemetrySettings)
	dependencies := server.Dependencies()

	require.Equal(t, expectedDependencies, dependencies)
}
