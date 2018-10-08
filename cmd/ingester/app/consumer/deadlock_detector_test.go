// Copyright (c) 2018 The Jaeger Authors.
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

package consumer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/testutils"
	"go.uber.org/zap"
)

func TestClosingSignalEmitted(t *testing.T) {
	mf := metrics.NewLocalFactory(0)
	l, _ := zap.NewDevelopment()
	f := newDeadlockDetector(mf, l, time.Millisecond)
	w := f.startMonitoringForPartition(1)
	assert.NotNil(t, <-w.closePartitionChannel())
	w.close()
}

func TestNoClosingSignalIfMessagesProcessedInInterval(t *testing.T) {
	mf := metrics.NewLocalFactory(0)
	l, _ := zap.NewDevelopment()
	f := newDeadlockDetector(mf, l, time.Second)
	f.start()
	defer f.close()

	w := f.startMonitoringForPartition(1)

	w.incrementMsgCount()
	assert.Zero(t, len(w.closePartitionChannel()))
	w.close()
}

func TestResetMsgCount(t *testing.T) {
	mf := metrics.NewLocalFactory(0)
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
	mf := metrics.NewLocalFactory(0)
	l, _ := zap.NewDevelopment()
	f := newDeadlockDetector(mf, l, time.Minute)

	assert.Panics(t, func() {
		f.panicFunc(1)
	})

	testutils.AssertCounterMetrics(t, mf, testutils.ExpectedMetric{
		Name:  "deadlockdetector.panic-issued",
		Tags:  map[string]string{"partition": "1"},
		Value: 1,
	})
}

func TestPanicForPartition(t *testing.T) {
	mf := metrics.NewLocalFactory(0)
	l, _ := zap.NewDevelopment()
	wg := sync.WaitGroup{}
	wg.Add(1)
	f := newDeadlockDetector(mf, l, time.Millisecond)
	f.panicFunc = func(partition int32) {
		wg.Done()
	}
	w := f.startMonitoringForPartition(1)
	wg.Wait()
	w.close()
}

func TestGlobalPanic(t *testing.T) {
	mf := metrics.NewLocalFactory(0)
	l, _ := zap.NewDevelopment()
	wg := sync.WaitGroup{}
	wg.Add(1)
	f := newDeadlockDetector(mf, l, time.Millisecond)
	f.panicFunc = func(partition int32) {
		wg.Done()
	}
	f.start()
	defer f.close()
	wg.Wait()
}
