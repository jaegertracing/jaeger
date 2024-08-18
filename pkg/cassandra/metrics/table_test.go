// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/testutils"
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
				"attempts|table=a_table": 1,
				"inserts|table=a_table":  1,
			},
			gauges: map[string]int64{
				"latency-ok|table=a_table.P999": 51,
				"latency-ok|table=a_table.P50":  51,
				"latency-ok|table=a_table.P75":  51,
				"latency-ok|table=a_table.P90":  51,
				"latency-ok|table=a_table.P95":  51,
				"latency-ok|table=a_table.P99":  51,
			},
		},
		{
			err: errors.New("some error"),
			counts: map[string]int64{
				"attempts|table=a_table": 1,
				"errors|table=a_table":   1,
			},
			gauges: map[string]int64{
				"latency-err|table=a_table.P999": 51,
				"latency-err|table=a_table.P50":  51,
				"latency-err|table=a_table.P75":  51,
				"latency-err|table=a_table.P90":  51,
				"latency-err|table=a_table.P95":  51,
				"latency-err|table=a_table.P99":  51,
			},
		},
	}
	for _, tc := range testCases {
		mf := metricstest.NewFactory(time.Second)
		tm := NewTable(mf, "a_table")
		tm.Emit(tc.err, 50*time.Millisecond)
		counts, gauges := mf.Snapshot()
		assert.Equal(t, tc.counts, counts)
		assert.Equal(t, tc.gauges, gauges)
		mf.Stop()
	}
}

func TestTableExec(t *testing.T) {
	testCases := []struct {
		q      insertQuery
		log    bool
		counts map[string]int64
	}{
		{
			q: insertQuery{},
			counts: map[string]int64{
				"attempts|table=a_table": 1,
				"inserts|table=a_table":  1,
			},
		},
		{
			q: insertQuery{
				str: "SELECT * FROM something",
				err: errors.New("failed"),
			},
			counts: map[string]int64{
				"attempts|table=a_table": 1,
				"errors|table=a_table":   1,
			},
		},
		{
			q: insertQuery{
				str: "SELECT * FROM something",
				err: errors.New("failed"),
			},
			log: true,
			counts: map[string]int64{
				"attempts|table=a_table": 1,
				"errors|table=a_table":   1,
			},
		},
	}

	for _, tc := range testCases {
		mf := metricstest.NewFactory(0)
		tm := NewTable(mf, "a_table")
		logger, logBuf := testutils.NewLogger()

		useLogger := logger
		if !tc.log {
			useLogger = nil
		}
		err := tm.Exec(tc.q, useLogger)
		if tc.q.err == nil {
			require.NoError(t, err)
			assert.Empty(t, logBuf.Bytes())
		} else {
			require.Error(t, err, tc.q.err.Error())
			if tc.log {
				assert.Equal(t, map[string]string{
					"level": "error",
					"msg":   "Failed to exec query",
					"query": "SELECT * FROM something",
					"error": "failed",
				}, logBuf.JSONLine(0))
			} else {
				assert.Empty(t, logBuf.Bytes())
			}
		}
		counts, _ := mf.Snapshot()
		assert.Equal(t, tc.counts, counts)
	}
}

type insertQuery struct {
	err error
	str string
}

func (q insertQuery) Exec() error {
	return q.err
}

func (q insertQuery) String() string {
	return q.str
}

func (insertQuery) ScanCAS(...any /* dest */) (bool, error) {
	return true, nil
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
