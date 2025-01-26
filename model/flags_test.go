// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func TestIsDebug(t *testing.T) {
	flags := model.Flags(0)
	flags.SetDebug()
	assert.True(t, flags.IsDebug())
	flags = model.Flags(0)
	assert.False(t, flags.IsDebug())

	flags = model.Flags(32)
	assert.False(t, flags.IsDebug())
	flags.SetDebug()
	assert.True(t, flags.IsDebug())
}

func TestIsFirehoseEnabled(t *testing.T) {
	flags := model.Flags(0)
	assert.False(t, flags.IsFirehoseEnabled())
	flags.SetDebug()
	flags.SetSampled()
	assert.False(t, flags.IsFirehoseEnabled())
	flags.SetFirehose()
	assert.True(t, flags.IsFirehoseEnabled())

	flags = model.Flags(8)
	assert.True(t, flags.IsFirehoseEnabled())
}

func TestIsSampled(t *testing.T) {
	flags := model.Flags(0)
	flags.SetSampled()
	assert.True(t, flags.IsSampled())
	flags = model.Flags(0)
	flags.SetDebug()
	assert.False(t, flags.IsSampled())
}
