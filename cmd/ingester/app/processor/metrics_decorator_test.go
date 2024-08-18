// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor/mocks"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

type fakeMsg struct{}

func (fakeMsg) Value() []byte {
	return nil
}

func TestProcess(t *testing.T) {
	p := &mocks.SpanProcessor{}
	msg := fakeMsg{}
	p.On("Process", msg).Return(nil)
	m := metricstest.NewFactory(0)
	proc := processor.NewDecoratedProcessor(m, p)

	proc.Process(msg)
	p.AssertExpectations(t)
	_, g := m.Snapshot()
	assert.Contains(t, g, "span-processor.latency.P90")
}

func TestProcessErr(t *testing.T) {
	p := &mocks.SpanProcessor{}
	msg := fakeMsg{}
	p.On("Process", msg).Return(errors.New("err"))
	m := metricstest.NewFactory(0)
	proc := processor.NewDecoratedProcessor(m, p)

	proc.Process(msg)
	p.AssertExpectations(t)
	c, g := m.Snapshot()
	assert.Contains(t, g, "span-processor.latency.P90")
	assert.Equal(t, int64(1), c["span-processor.errors"])
}
