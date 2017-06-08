// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package testutils

import (
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
		_ = <-start
		logger.Info("test")
		finish.Done()
	}()

	go func() {
		_ = <-start
		buffer.Lines()
		buffer.Stripped()
		finish.Done()
	}()

	close(start)
	finish.Wait()
}
