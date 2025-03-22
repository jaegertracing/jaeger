// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package offset

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestHandleReset(t *testing.T) {
	offset := int64(1498)
	minOffset := offset - 1

	m := metricstest.NewFactory(0)

	var wg sync.WaitGroup
	wg.Add(1)
	var captureOffset int64
	fakeMarker := func(offset int64) {
		captureOffset = offset
		wg.Done()
	}
	manager := NewManager(minOffset, fakeMarker, "test_topic", 1, m)
	manager.Start()

	manager.MarkOffset(offset)
	wg.Wait()
	manager.Close()

	assert.Equal(t, offset, captureOffset)
	cnt, g := m.Snapshot()
	assert.Equal(t, int64(1), cnt["offset-commits-total|partition=1|topic=test_topic"])
	assert.Equal(t, int64(offset), g["last-committed-offset|partition=1|topic=test_topic"])
}

func TestCache(t *testing.T) {
	offset := int64(1498)

	fakeMarker := func(_ /* offset */ int64) {
		assert.Fail(t, "Shouldn't mark cached offset")
	}
	manager := NewManager(offset, fakeMarker, "test_topic", 1, metrics.NullFactory)
	manager.Start()
	time.Sleep(resetInterval + 50)
	manager.MarkOffset(offset)
	manager.Close()
}
