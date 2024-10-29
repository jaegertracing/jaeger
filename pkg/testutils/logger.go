// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

// NewLogger creates a new zap.Logger backed by a zaptest.Buffer, which is also returned.
func NewLogger() (*zap.Logger, *Buffer) {
	core, buf := newRecordingCore()
	logger := zap.New(core, zap.WithFatalHook(zapcore.WriteThenPanic))
	return logger, buf
}

func newRecordingCore() (zapcore.Core, *Buffer) {
	encoder := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
		MessageKey:     "msg",
		LevelKey:       "level",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	})
	buf := &Buffer{}
	return zapcore.NewCore(encoder, buf, zapcore.DebugLevel), buf
}

// NewEchoLogger is similar to NewLogger, but the logs are also echoed to t.Log.
func NewEchoLogger(t *testing.T) (*zap.Logger, *Buffer) {
	core, buf := newRecordingCore()
	echo := zaptest.NewLogger(t).Core()
	logger := zap.New(zapcore.NewTee(core, echo))
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

// String overwrites zaptest.Buffer.String() to make it thread safe
func (b *Buffer) String() string {
	b.RLock()
	defer b.RUnlock()
	return b.Buffer.String()
}

// Write overwrites zaptest.Buffer.bytes.Buffer.Write() to make it thread safe
func (b *Buffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}

var LogMatcher = func(occurrences int, subStr string, logs []string) (bool, string) {
	errMsg := fmt.Sprintf("subStr '%s' does not occur %d time(s) in %v", subStr, occurrences, logs)
	if len(logs) < occurrences {
		return false, errMsg
	}

	// Count occurrences in parallel
	var count int
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, log := range logs {
		wg.Add(1)
		go func(log string) {
			defer wg.Done()
			if strings.Contains(log, subStr) {
				mu.Lock()
				count++
				mu.Unlock()
			}
		}(log)
	}
	wg.Wait()

	if count >= occurrences {
		return true, ""
	}
	return false, errMsg
}
