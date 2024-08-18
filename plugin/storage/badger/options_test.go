// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
)

func TestDefaultOptionsParsing(t *testing.T) {
	opts := NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{})
	opts.InitFromViper(v, zap.NewNop())

	assert.True(t, opts.GetPrimary().Ephemeral)
	assert.False(t, opts.GetPrimary().SyncWrites)
	assert.Equal(t, time.Duration(72*time.Hour), opts.GetPrimary().SpanStoreTTL)
}

func TestParseOptions(t *testing.T) {
	opts := NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=false",
		"--badger.consistency=true",
		"--badger.directory-key=/var/lib/badger",
		"--badger.directory-value=/mnt/slow/badger",
		"--badger.span-store-ttl=168h",
	})
	opts.InitFromViper(v, zap.NewNop())

	assert.False(t, opts.GetPrimary().Ephemeral)
	assert.True(t, opts.GetPrimary().SyncWrites)
	assert.Equal(t, time.Duration(168*time.Hour), opts.GetPrimary().SpanStoreTTL)
	assert.Equal(t, "/var/lib/badger", opts.GetPrimary().KeyDirectory)
	assert.Equal(t, "/mnt/slow/badger", opts.GetPrimary().ValueDirectory)
	assert.False(t, opts.GetPrimary().ReadOnly)
}

func TestReadOnlyOptions(t *testing.T) {
	opts := NewOptions("badger")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{
		"--badger.read-only=true",
	})
	opts.InitFromViper(v, zap.NewNop())
	assert.True(t, opts.GetPrimary().ReadOnly)
}
