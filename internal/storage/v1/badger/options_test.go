// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"flag"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDefaultConfigParsing(t *testing.T) {
	cfg := DefaultConfig()

	assert.True(t, cfg.Ephemeral)
	assert.False(t, cfg.SyncWrites)
	assert.Equal(t, time.Duration(72*time.Hour), cfg.TTL.Spans)
	assert.Equal(t, SpanEncodingProtobuf, cfg.SpanEncoding)
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

func TestSpanEncodingConfigOption(t *testing.T) {
	cfg := DefaultConfig()
	flags := flag.NewFlagSet("badger", flag.ContinueOnError)
	cfg.AddFlags(flags)
	v := viper.New()
	require.NoError(t, flags.Parse([]string{"--badger.span-encoding=json"}))
	v.Set(prefix+suffixSpanEncoding, flags.Lookup(prefix+suffixSpanEncoding).Value.String())
	cfg.InitFromViper(v, zap.NewNop())
	assert.Contains(t, []string{SpanEncodingJSON}, cfg.SpanEncoding)
}
