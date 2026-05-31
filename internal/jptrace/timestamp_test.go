// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func TestTimeToUnixNano(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want uint64
	}{
		{name: "zero time", in: time.Time{}, want: 0},
		{name: "pre-epoch", in: time.Unix(-1, 0), want: 0},
		{name: "epoch", in: time.Unix(0, 0).UTC(), want: 0},
		{name: "positive", in: time.Unix(0, 1_000_000_000).UTC(), want: 1_000_000_000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, jptrace.TimeToUnixNano(tc.in))
		})
	}
}

func TestUnixNanoToTime(t *testing.T) {
	tests := []struct {
		name string
		in   uint64
		want time.Time
	}{
		{name: "zero", in: 0, want: time.Time{}},
		{name: "positive", in: 1_000_000_000, want: time.Unix(1, 0).UTC()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, jptrace.UnixNanoToTime(tc.in))
		})
	}
}
