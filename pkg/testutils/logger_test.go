// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewLogger(t *testing.T) {
	t.Parallel()
	logger, log := NewLogger()
	logger.Warn("hello", zap.String("x", "y"))

	assert.Equal(t, `{"level":"warn","msg":"hello","x":"y"}`, log.Lines()[0])
	assert.Equal(t, map[string]string{
		"level": "warn",
		"msg":   "hello",
		"x":     "y",
	}, log.JSONLine(0))
}

func TestNewEchoLogger(t *testing.T) {
	t.Parallel()
	logger, _ := NewEchoLogger(t)
	logger.Warn("hello", zap.String("x", "y"))
}

func TestJSONLineError(t *testing.T) {
	t.Parallel()
	log := &Buffer{}
	log.WriteString("bad-json\n")
	_, ok := log.JSONLine(0)["error"]
	assert.True(t, ok, "must have 'error' key")
}

// NB. Run with -race to ensure no race condition
func TestRaceCondition(*testing.T) {
	logger, buffer := NewLogger()

	start := make(chan struct{})
	finish := sync.WaitGroup{}
	finish.Add(2)

	go func() {
		<-start
		logger.Info("test")
		finish.Done()
	}()

	go func() {
		<-start
		buffer.Lines()
		buffer.Stripped()
		_ = buffer.String()
		finish.Done()
	}()

	close(start)
	finish.Wait()
}

func TestLogMatcher(t *testing.T) {
	t.Parallel()
	tests := []struct {
		occurrences int
		subStr      string
		logs        []string
		expected    bool
		errMsg      string
	}{
		{occurrences: 1, expected: false, errMsg: "subStr '' does not occur 1 time(s) in []"},
		{occurrences: 1, subStr: "hi", logs: []string{"hi"}, expected: true},
		{occurrences: 3, subStr: "hi", logs: []string{"hi", "hi"}, expected: false, errMsg: "subStr 'hi' does not occur 3 time(s) in [hi hi]"},
		{occurrences: 3, subStr: "hi", logs: []string{"hi", "hi", "hi"}, expected: true},
		{occurrences: 1, subStr: "hi", logs: []string{"bye", "bye"}, expected: false, errMsg: "subStr 'hi' does not occur 1 time(s) in [bye bye]"},
	}
	for i, tt := range tests {
		test := tt
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			t.Parallel()
			match, errMsg := LogMatcher(test.occurrences, test.subStr, test.logs)
			assert.Equal(t, test.expected, match)
			assert.Equal(t, test.errMsg, errMsg)
		})
	}
}
