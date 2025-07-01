// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestClosingSignalEmitted(t *testing.T) {
	mf := metricstest.NewFactory(0)
	l, _ := zap.NewDevelopment()
	f := newDeadlockDetector(mf, l, time.Millisecond)
	w := f.startMonitoringForPartition(1)
	assert.NotNil(t, <-w.closePartitionChannel())
	w.close()
}

func TestNoClosingSignalIfMessagesProcessedInInterval(t *testing.T) {
	mf := metricstest.NewFactory(0)
	l, _ := zap.NewDevelopment()
	f := newDeadlockDetector(mf, l, time.Second)
	f.start()
	defer f.close()

	w := f.startMonitoringForPartition(1)

	w.incrementMsgCount()
	assert.Empty(t, w.closePartitionChannel())
	w.close()
}

func TestResetMsgCount(t *testing.T) {
	mf := metricstest.NewFactory(0)
	l, _ := zap.NewDevelopment()
	f := newDeadlockDetector(mf, l, 50*time.Millisecond)
	f.start()
	defer f.close()
	w := f.startMonitoringForPartition(1)
	w.incrementMsgCount()
	time.Sleep(75 * time.Millisecond)
	// Resets happen after every ticker interval
	w.close()
	assert.Zero(t, atomic.LoadUint64(w.msgConsumed))
}

func TestPanicFunc(t *testing.T) {
	mf := metricstest.NewFactory(0)
	l, _ := zap.NewDevelopment()
	f := newDeadlockDetector(mf, l, time.Minute)

	assert.Panics(t, func() {
		f.panicFunc(1)
	})

	mf.AssertCounterMetrics(t, metricstest.ExpectedMetric{
		Name:  "deadlockdetector.panic-issued",
		Tags:  map[string]string{"partition": "1"},
		Value: 1,
	})
}

func TestPanicForPartition(*testing.T) {
	l, _ := zap.NewDevelopment()
	wg := sync.WaitGroup{}
	wg.Add(1)
	d := deadlockDetector{
		metricsFactory: metricstest.NewFactory(0),
		logger:         l,
		interval:       1,
		panicFunc: func(_ /* partition */ int32) {
			wg.Done()
		},
	}

	d.startMonitoringForPartition(1)
	wg.Wait()
}

func TestGlobalPanic(*testing.T) {
	l, _ := zap.NewDevelopment()
	wg := sync.WaitGroup{}
	wg.Add(1)
	d := deadlockDetector{
		metricsFactory: metricstest.NewFactory(0),
		logger:         l,
		interval:       1,
		panicFunc: func(_ /* partition */ int32) {
			wg.Done()
		},
	}

	d.start()
	wg.Wait()
}

func TestNoGlobalPanicIfDeadlockDetectorDisabled(t *testing.T) {
	l, _ := zap.NewDevelopment()
	d := deadlockDetector{
		metricsFactory: metricstest.NewFactory(0),
		logger:         l,
		interval:       0,
		panicFunc: func(_ /* partition */ int32) {
			t.Error("Should not panic when deadlock detector is disabled")
		},
	}

	d.start()

	time.Sleep(100 * time.Millisecond)

	d.close()
}

func TestNoPanicForPartitionIfDeadlockDetectorDisabled(t *testing.T) {
	l, _ := zap.NewDevelopment()
	d := deadlockDetector{
		metricsFactory: metricstest.NewFactory(0),
		logger:         l,
		interval:       0,
		panicFunc: func(_ /* partition */ int32) {
			t.Error("Should not panic when deadlock detector is disabled")
		},
	}

	w := d.startMonitoringForPartition(1)
	time.Sleep(100 * time.Millisecond)

	w.close()
}

// same as TestNoClosingSignalIfMessagesProcessedInInterval but with disabled deadlock detector
func TestApiCompatibilityWhenDeadlockDetectorDisabled(t *testing.T) {
	mf := metricstest.NewFactory(0)
	l, _ := zap.NewDevelopment()
	f := newDeadlockDetector(mf, l, 0)
	f.start()
	defer f.close()

	w := f.startMonitoringForPartition(1)

	w.incrementMsgCount()
	w.incrementAllPartitionMsgCount()
	assert.Empty(t, w.closePartitionChannel())
	w.close()
}
