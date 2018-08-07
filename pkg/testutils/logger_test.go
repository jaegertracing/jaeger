// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutils

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewLogger(t *testing.T) {
	logger, log := NewLogger()
	logger.Warn("hello", zap.String("x", "y"))

	assert.Equal(t, `{"level":"warn","msg":"hello","x":"y"}`, log.Lines()[0])
	assert.Equal(t, map[string]string{
		"level": "warn",
		"msg":   "hello",
		"x":     "y",
	}, log.JSONLine(0))
}

func TestJSONLineError(t *testing.T) {
	log := &Buffer{}
	log.WriteString("bad-json\n")
	_, ok := log.JSONLine(0)["error"]
	assert.True(t, ok, "must have 'error' key")
}

// NB. Run with -race to ensure no race condition
func TestRaceCondition(t *testing.T) {
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
	tests := []struct {
		occurences int
		subStr     string
		logs       []string
		expected   bool
	}{
		{occurences: 1, expected: false},
		{occurences: 1, subStr: "hi", logs: []string{"hi"}, expected: true},
		{occurences: 3, subStr: "hi", logs: []string{"hi", "hi"}, expected: false},
		{occurences: 3, subStr: "hi", logs: []string{"hi", "hi", "hi"}, expected: true},
	}
	for i, tt := range tests {
		test := tt
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			assert.Equal(t, test.expected, LogMatcher(test.occurences, test.subStr, test.logs))
		})
	}
}
