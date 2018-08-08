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

package offset

import (
	"strconv"
	"sync"
	"time"

	"github.com/uber/jaeger-lib/metrics"
)

const (
	resetInterval = 100 * time.Millisecond
)

// Manager accepts kafka offsets and commits them using the provided kafka consumer
//
// The Manager is designed to be used in a scenario where the consumption of kafka offsets
// is decoupled from the processing of offsets asynchronously via goroutines. This breaks the
// ordering guarantee which could result in the completion of processing of an earlier message
// after the processing of a later message.
//
// It assumes that Kafka offsets are sequential and monotonically increasing[1], and maintains
// sorted lists of offsets per partition.
//
// [1] https://kafka.apache.org/0100/javadoc/index.html?org/apache/kafka/clients/consumer/KafkaConsumer.html
type Manager struct {
	markOffsetFunction  MarkOffset
	offsetCommitCount   metrics.Counter
	lastCommittedOffset metrics.Gauge
	list                *ConcurrentList
	close               chan struct{}
	isClosed            sync.WaitGroup
}

// MarkOffset is a func that marks offsets in Kafka
type MarkOffset func(offset int64)

// NewManager creates a new Manager
func NewManager(minOffset int64, markOffset MarkOffset, partition int32, factory metrics.Factory) *Manager {
	return &Manager{
		markOffsetFunction:  markOffset,
		close:               make(chan struct{}),
		offsetCommitCount:   factory.Counter("offset-commit-count", map[string]string{"partition": strconv.Itoa(int(partition))}),
		lastCommittedOffset: factory.Gauge("offset-commit", map[string]string{"partition": strconv.Itoa(int(partition))}),
		list:                newConcurrentList(minOffset),
	}
}

// MarkOffset marks the offset of a consumer message
func (m *Manager) MarkOffset(offset int64) {
	m.list.insert(offset)
}

// Start starts the Manager
func (m *Manager) Start() {
	m.isClosed.Add(1)
	go func() {
		for {
			select {
			case <-time.After(resetInterval):
				offset := m.list.setToHighestContiguous()
				m.offsetCommitCount.Inc(1)
				m.lastCommittedOffset.Update(offset)
				m.markOffsetFunction(offset)
			case <-m.close:
				m.isClosed.Done()
				return
			}
		}
	}()
}

// Close closes the Manager
func (m *Manager) Close() error {
	close(m.close)
	m.isClosed.Wait()
	return nil
}
