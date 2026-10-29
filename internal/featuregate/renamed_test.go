// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package featuregate

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collectorfeaturegate "go.opentelemetry.io/collector/featuregate"
)

func TestRenamedGate(t *testing.T) {
	tests := []struct {
		name           string
		stage          collectorfeaturegate.Stage
		currentEnabled bool
		legacyEnabled  bool
		expected       bool
		expectWarning  bool
	}{
		{name: "Alpha defaults disabled", stage: collectorfeaturegate.StageAlpha},
		{name: "Alpha current enabled", stage: collectorfeaturegate.StageAlpha, currentEnabled: true, expected: true},
		{name: "Alpha legacy enabled", stage: collectorfeaturegate.StageAlpha, legacyEnabled: true, expected: true, expectWarning: true},
		{name: "Alpha both enabled", stage: collectorfeaturegate.StageAlpha, currentEnabled: true, legacyEnabled: true, expected: true},
		{name: "Beta defaults enabled", stage: collectorfeaturegate.StageBeta, currentEnabled: true, legacyEnabled: true, expected: true},
		{name: "Beta current disabled", stage: collectorfeaturegate.StageBeta, legacyEnabled: true},
		{name: "Beta legacy disabled", stage: collectorfeaturegate.StageBeta, currentEnabled: true, expectWarning: true},
		{name: "Beta both disabled", stage: collectorfeaturegate.StageBeta},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := collectorfeaturegate.NewRegistry()
			current := registry.MustRegister("jaeger.test", test.stage)
			legacy := registry.MustRegister("test", test.stage)
			require.NoError(t, registry.Set(current.ID(), test.currentEnabled))
			require.NoError(t, registry.Set(legacy.ID(), test.legacyEnabled))

			gate := NewRenamedGate(current, legacy)
			var warnings bytes.Buffer
			gate.warnings = &warnings

			assert.Equal(t, "jaeger.test", gate.ID())
			assert.Equal(t, "test", gate.LegacyID())
			assert.Equal(t, test.expected, gate.IsEnabled())
			assert.Equal(t, test.expectWarning, warnings.Len() > 0)
			if test.expectWarning {
				assert.Equal(t, "Feature gate \"test\" has been renamed to \"jaeger.test\"; the legacy ID will be removed in a future release.\n", warnings.String())
			}
			warningLength := warnings.Len()
			assert.Equal(t, test.expected, gate.IsEnabled())
			assert.Equal(t, warningLength, warnings.Len())
		})
	}
}

func TestNewRenamedGatePanics(t *testing.T) {
	t.Run("different stages", func(t *testing.T) {
		registry := collectorfeaturegate.NewRegistry()
		current := registry.MustRegister("jaeger.test", collectorfeaturegate.StageAlpha)
		legacy := registry.MustRegister("test", collectorfeaturegate.StageBeta)
		assert.PanicsWithValue(t, "renamed feature gate IDs must use the same stage", func() {
			NewRenamedGate(current, legacy)
		})
	})

	t.Run("unsupported stage", func(t *testing.T) {
		registry := collectorfeaturegate.NewRegistry()
		current := registry.MustRegister("jaeger.test", collectorfeaturegate.StageStable, collectorfeaturegate.WithRegisterToVersion("v3.0.0"))
		legacy := registry.MustRegister("test", collectorfeaturegate.StageStable, collectorfeaturegate.WithRegisterToVersion("v3.0.0"))
		assert.PanicsWithValue(t, "only Alpha and Beta feature gates can be renamed", func() {
			NewRenamedGate(current, legacy)
		})
	})
}
