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
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/zap"
)

func TestNewTextLogger(t *testing.T) {
	logger, log := NewLogger(false)
	logger.Warn("hello", zap.String("x", "y"))
	v := string(log.Bytes())
	assert.Equal(t, "[W] hello x=y\n", v)
}

func TestNewJSONLogger(t *testing.T) {
	logger, log := NewLogger(true)
	logger.Warn("hello", zap.String("x", "y"))

	data := make(map[string]string)
	require.NoError(t, json.Unmarshal(log.Bytes(), &data))
	delete(data, "time")
	assert.Equal(t, map[string]string{
		"level": "warn",
		"msg":   "hello",
		"x":     "y",
	}, data)
}
