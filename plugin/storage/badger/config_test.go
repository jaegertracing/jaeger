// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultConfigParsing(t *testing.T) {
	cfg := NewConfig()
	v, command := config.Viperize(cfg.AddFlags)
	command.ParseFlags([]string{})
	cfg.InitFromViper(v, zap.NewNop())

	assert.True(t, cfg.Ephemeral)
	assert.False(t, cfg.SyncWrites)
	assert.Equal(t, time.Duration(72*time.Hour), cfg.TTL.Spans)
}

func TestParseConfig(t *testing.T) {
	cfg := NewConfig()
	v, command := config.Viperize(cfg.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=false",
		"--badger.consistency=true",
		"--badger.directory-key=/var/lib/badger",
		"--badger.directory-value=/mnt/slow/badger",
		"--badger.span-store-ttl=168h",
	})
	cfg.InitFromViper(v, zap.NewNop())

	assert.False(t, cfg.Ephemeral)
	assert.True(t, cfg.SyncWrites)
	assert.Equal(t, time.Duration(168*time.Hour), cfg.TTL.Spans)
	assert.Equal(t, "/var/lib/badger", cfg.Directories.Keys)
	assert.Equal(t, "/mnt/slow/badger", cfg.Directories.Values)
	assert.False(t, cfg.ReadOnly)
}

func TestReadOnlyConfig(t *testing.T) {
	cfg := NewConfig()
	v, command := config.Viperize(cfg.AddFlags)
	command.ParseFlags([]string{
		"--badger.read-only=true",
	})
	cfg.InitFromViper(v, zap.NewNop())
	assert.True(t, cfg.ReadOnly)
}

func TestValidate_DoesNotReturnErrorWhenValid(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "non-required fields not set",
			cfg:  &Config{},
		},
		{
			name: "all fields are set",
			cfg: &Config{
				TTL: TTL{
					Spans: time.Second,
				},
				Directories: Directories{
					Keys:   "some-key-directory",
					Values: "some-values-directory",
				},
				Ephemeral:             false,
				SyncWrites:            false,
				MaintenanceInterval:   time.Second,
				MetricsUpdateInterval: time.Second,
				ReadOnly:              false,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.cfg.Validate()
			require.NoError(t, err)
		})
	}
}
