// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor_test

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	mockProcessor "github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
)

type fakeMessage struct{}

func (fakeMessage) Value() []byte {
	return nil
}

func TestNewParallelProcessor(t *testing.T) {
	msg := &fakeMessage{}
	mp := &mockProcessor.SpanProcessor{}
	mp.On("Process", msg).Return(nil)

	pp := processor.NewParallelProcessor(mp, 1, zap.NewNop())
	pp.Start()

	pp.Process(msg)
	time.Sleep(100 * time.Millisecond)
	pp.Close()

	mp.AssertExpectations(t)
}
