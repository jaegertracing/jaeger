// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
)

func TestRunBackwardCompatibilityTests_SkipsWithoutEnv(t *testing.T) {
	t.Setenv(oldBinaryEnvVar, "")
	t.Setenv(oldConfigDirEnvVar, "")

	// Verify that runBackwardCompatibilityTests skips without panicking when env vars are missing
	runBackwardCompatibilityTests(t, "memory", E2EStorageIntegration{}, compatScenario{
		Name:         "feature gates disabled on both old writer and new reader",
		Capabilities: capabilities.Memory(),
	})
}

func TestMustSkipForWriterVersion(t *testing.T) {
	tests := []struct {
		name             string
		writerVersion    string
		minWriterVersion string
		wantSkip         bool
		wantContains     string
	}{
		{
			name:             "no minimum never skips",
			writerVersion:    "v2.23.0",
			minWriterVersion: "",
			wantSkip:         false,
		},
		{
			name:             "main baseline never skips for a minimum",
			writerVersion:    oldVersionMain,
			minWriterVersion: "v2.24.0",
			wantSkip:         false,
		},
		{
			name:             "missing writer version skips when a minimum is set",
			writerVersion:    "",
			minWriterVersion: "v2.24.0",
			wantSkip:         true,
			wantContains:     oldVersionEnvVar,
		},
		{
			name:             "older release skips",
			writerVersion:    "v2.23.0",
			minWriterVersion: "v2.24.0",
			wantSkip:         true,
			wantContains:     "predates",
		},
		{
			name:             "same release runs",
			writerVersion:    "v2.24.0",
			minWriterVersion: "v2.24.0",
			wantSkip:         false,
		},
		{
			name:             "newer release runs",
			writerVersion:    "v2.25.0",
			minWriterVersion: "v2.24.0",
			wantSkip:         false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := mustSkipForWriterVersion(t, tt.writerVersion, tt.minWriterVersion)
			assert.Equal(t, tt.wantSkip, skip)
			if tt.wantSkip {
				assert.Contains(t, reason, tt.wantContains)
				return
			}
			assert.Empty(t, reason)
		})
	}
}

func TestCompareReleaseVersions(t *testing.T) {
	tests := []struct {
		name    string
		a, b    string
		want    int
		wantErr bool
	}{
		{name: "equal", a: "v2.24.0", b: "v2.24.0", want: 0},
		{name: "less", a: "v2.23.0", b: "v2.24.0", want: -1},
		{name: "greater", a: "v2.25.1", b: "v2.24.0", want: 1},
		{name: "patch less", a: "v2.24.0", b: "v2.24.1", want: -1},
		{name: "missing v", a: "2.24.0", b: "v2.24.0", wantErr: true},
		{name: "too few parts", a: "v2.24", b: "v2.24.0", wantErr: true},
		{name: "non numeric", a: "v2.x.0", b: "v2.24.0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compareReleaseVersions(tt.a, tt.b)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWriteConfigFile(t *testing.T) {
	oldDir := "/tmp/jaeger-old/cmd/jaeger"
	suiteConfig := "../../config-clickhouse.yaml"

	t.Run("defaults to earlier checkout copy", func(t *testing.T) {
		got := writeConfigFile(suiteConfig, oldDir, "")
		assert.Equal(t, filepath.Join(oldDir, "config-clickhouse.yaml"), got)
	})

	t.Run("generated write config wins", func(t *testing.T) {
		generated := "/tmp/generated/config-clickhouse-map.yaml"
		got := writeConfigFile(suiteConfig, oldDir, generated)
		assert.Equal(t, generated, got)
	})
}

func TestRunBackwardCompatibilityTests_SkipsPredatingWriter(t *testing.T) {
	t.Setenv(oldBinaryEnvVar, "/bin/true")
	t.Setenv(oldConfigDirEnvVar, t.TempDir())
	t.Setenv(oldVersionEnvVar, "v2.23.0")

	// The scenario must skip before e2eInitialize tries to start a real Jaeger.
	runBackwardCompatibilityTests(t, "memory", E2EStorageIntegration{
		ConfigFile: "config.yaml",
	}, compatScenario{
		Name:             "requires a newer writer",
		MinWriterVersion: "v2.24.0",
		Capabilities:     capabilities.Memory(),
	})
}
