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
	f := newSeppukuFactory(mf, l, time.Millisecond)
	w := f.startMonitoringForPartition(1)
	assert.NotNil(t, <-w.getClosePartition())
	w.close()
}

func TestNoClosingSignalIfMessagesProcessedInInterval(t *testing.T) {
	mf := metrics.NewLocalFactory(0)
	l, _ := zap.NewDevelopment()
	f := newSeppukuFactory(mf, l, time.Second)
	w := f.startMonitoringForPartition(1)

	w.incrementMsgCount()
	assert.Zero(t, len(w.getClosePartition()))
	w.close()
}

func TestResetMsgCount(t *testing.T) {
	mf := metrics.NewLocalFactory(0)
	l, _ := zap.NewDevelopment()
	f := newSeppukuFactory(mf, l, 50*time.Millisecond)
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

	assert.Panics(t, func() {
		f := newSeppukuFactory(mf, l, 1)
		f.panicFunc(1)
	})

	testutils.AssertCounterMetrics(t, mf, testutils.ExpectedMetric{
		Name:  "seppuku",
		Tags:  map[string]string{"partition": "1"},
		Value: 1,
	})
}

func TestSeppuku(t *testing.T) {
	mf := metrics.NewLocalFactory(0)
	l, _ := zap.NewDevelopment()
	f := newSeppukuFactory(mf, l, 1)
	wg := sync.WaitGroup{}
	wg.Add(1)
	f.panicFunc = func(partition int32) {
		wg.Done()
	}
	w := f.startMonitoringForPartition(1)
	wg.Wait()
	w.close()
}
