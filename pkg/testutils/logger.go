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
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

// NewLogger creates a new zap.Logger backed by a zaptest.Buffer, which is also returned.
func NewLogger() (*zap.Logger, *Buffer) {
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	})
	buf := &Buffer{}
	logger := zap.New(
		zapcore.NewCore(encoder, buf, zapcore.DebugLevel),
	)
	return logger, buf
}

// Buffer wraps zaptest.Buffer and provides convenience method JSONLine(n)
type Buffer struct {
	sync.RWMutex
	zaptest.Buffer
}

// JSONLine reads n-th line from the buffer and converts it to JSON.
func (b *Buffer) JSONLine(n int) map[string]string {
	data := make(map[string]string)
	line := b.Lines()[n]
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		return map[string]string{
			"error": err.Error(),
		}
	}
	return data
}

// NB. the below functions overwrite the existing functions so that logger is threadsafe.
// This is not that fragile given how if the API were to change underneath in zap, the overwritten
// function will fail to compile.

// Lines overwrites zaptest.Buffer.Lines() to make it thread safe
func (b *Buffer) Lines() []string {
	b.RLock()
	defer b.RUnlock()
	return b.Buffer.Lines()
}

// Stripped overwrites zaptest.Buffer.Stripped() to make it thread safe
func (b *Buffer) Stripped() string {
	b.RLock()
	defer b.RUnlock()
	return b.Buffer.Stripped()
}

// Write overwrites zaptest.Buffer.bytes.Buffer.Write() to make it thread safe
func (b *Buffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}
