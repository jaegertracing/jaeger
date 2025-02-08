// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstoremetrics

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestTableEmit(t *testing.T) {
	testCases := []struct {
		err    error
		counts map[string]int64
		gauges map[string]int64
	}{
		{
			err: nil,
			counts: map[string]int64{
				"a_table.attempts": 1,
				"a_table.inserts":  1,
			},
			gauges: map[string]int64{
				"a_table.latency-ok.P999": 51,
				"a_table.latency-ok.P50":  51,
				"a_table.latency-ok.P75":  51,
				"a_table.latency-ok.P90":  51,
				"a_table.latency-ok.P95":  51,
				"a_table.latency-ok.P99":  51,
			},
		},
		{
			err: errors.New("some error"),
			counts: map[string]int64{
				"a_table.attempts": 1,
				"a_table.errors":   1,
			},
			gauges: map[string]int64{
				"a_table.latency-err.P999": 51,
				"a_table.latency-err.P50":  51,
				"a_table.latency-err.P75":  51,
				"a_table.latency-err.P90":  51,
				"a_table.latency-err.P95":  51,
				"a_table.latency-err.P99":  51,
			},
		},
	}
	for _, tc := range testCases {
		mf := metricstest.NewFactory(time.Second)
		tm := NewWriter(mf, "a_table")
		tm.Emit(tc.err, 50*time.Millisecond)
		counts, gauges := mf.Snapshot()
		assert.Equal(t, tc.counts, counts)
		assert.Equal(t, tc.gauges, gauges)
		mf.Stop()
	}
}
