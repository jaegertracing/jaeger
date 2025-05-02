// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestOptionsWithDefaultFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}
	o.AddFlags(&c)

	assert.Equal(t, "localhost:16686", o.QueryGRPCHostPort)
	assert.Equal(t, "/tmp", o.OutputDir)
	assert.False(t, o.HashStandardTags)
	assert.False(t, o.HashCustomTags)
	assert.False(t, o.HashLogs)
	assert.False(t, o.HashProcess)
	assert.Equal(t, -1, o.MaxSpansCount)
	assert.Equal(t, int64(0), o.StartTime)
	assert.Equal(t, int64(0), o.EndTime)
}

func TestOptionsWithFlags(t *testing.T) {
	o := Options{}
	c := cobra.Command{}

	o.AddFlags(&c)
	c.ParseFlags([]string{
		"--query-host-port=192.168.1.10:16686",
		"--output-dir=/data",
		"--trace-id=6ef2debb698f2f7c",
		"--hash-standard-tags",
		"--hash-custom-tags",
		"--hash-logs",
		"--hash-process",
		"--max-spans-count=100",
		"--start-time=1",
		"--end-time=2",
	})

	assert.Equal(t, "192.168.1.10:16686", o.QueryGRPCHostPort)
	assert.Equal(t, "/data", o.OutputDir)
	assert.Equal(t, "6ef2debb698f2f7c", o.TraceID)
	assert.True(t, o.HashStandardTags)
	assert.True(t, o.HashCustomTags)
	assert.True(t, o.HashLogs)
	assert.True(t, o.HashProcess)
	assert.Equal(t, 100, o.MaxSpansCount)
	assert.Equal(t, int64(1), o.StartTime)
	assert.Equal(t, int64(2), o.EndTime)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
