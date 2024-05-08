// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagereceiver

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap/confmaptest"
)

func TestLoadConfig(t *testing.T) {
	t.Parallel()

	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)

	tests := []struct {
		id          component.ID
		expected    component.Config
		expectedErr error
	}{
		{
			id:          component.NewIDWithName(componentType, ""),
			expectedErr: errors.New("non zero value required"),
		},
		{
			id: component.NewIDWithName(componentType, "defaults"),
			expected: &Config{
				TraceStorage: "storage",
				PullInterval: 0,
			},
		},
		{
			id: component.NewIDWithName(componentType, "filled"),
			expected: &Config{
				TraceStorage: "storage",
				PullInterval: 2 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.id.String(), func(t *testing.T) {
			factory := NewFactory()
			cfg := factory.CreateDefaultConfig()

			sub, err := cm.Sub(tt.id.String())
			require.NoError(t, err)
			require.NoError(t, component.UnmarshalConfig(sub, cfg))

			if tt.expectedErr != nil {
				require.ErrorContains(t, component.ValidateConfig(cfg), tt.expectedErr.Error())
			} else {
				require.NoError(t, component.ValidateConfig(cfg))
				assert.Equal(t, tt.expected, cfg)
			}
		})
	}
}
