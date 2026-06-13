// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultConfigParsing(t *testing.T) {
	cfg := DefaultConfig()

	assert.True(t, cfg.Ephemeral)
	assert.False(t, cfg.SyncWrites)
	assert.Equal(t, time.Duration(72*time.Hour), cfg.TTL.Spans)
}

func TestParseConfig(t *testing.T) {
	cfg := &Config{
		Ephemeral:  false,
		SyncWrites: true,
		TTL: TTL{
			Spans: 168 * time.Hour,
		},
		Directories: Directories{
			Keys:   "/var/lib/badger",
			Values: "/mnt/slow/badger",
		},
		ReadOnly: false,
	}

	assert.False(t, cfg.Ephemeral)
	assert.True(t, cfg.SyncWrites)
	assert.Equal(t, time.Duration(168*time.Hour), cfg.TTL.Spans)
	assert.Equal(t, "/var/lib/badger", cfg.Directories.Keys)
	assert.Equal(t, "/mnt/slow/badger", cfg.Directories.Values)
	assert.False(t, cfg.ReadOnly)
}

func TestReadOnlyConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReadOnly = true
	assert.True(t, cfg.ReadOnly)
}
