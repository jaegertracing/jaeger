// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package offset

import (
	"strconv"
	"sync"
	"time"

	"github.com/jaegertracing/jaeger/internal/metrics"
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
	minOffset           int64
	list                *ConcurrentList
	close               chan struct{}
	isClosed            sync.WaitGroup
}

// MarkOffset is a func that marks offsets in Kafka
type MarkOffset func(offset int64)

// NewManager creates a new Manager
func NewManager(
	minOffset int64,
	markOffset MarkOffset,
	topic string,
	partition int32,
	factory metrics.Factory,
) *Manager {
	tags := map[string]string{
		"topic":     topic,
		"partition": strconv.Itoa(int(partition)),
	}
	return &Manager{
		markOffsetFunction:  markOffset,
		close:               make(chan struct{}),
		offsetCommitCount:   factory.Counter(metrics.Options{Name: "offset-commits-total", Tags: tags}),
		lastCommittedOffset: factory.Gauge(metrics.Options{Name: "last-committed-offset", Tags: tags}),
		list:                newConcurrentList(minOffset),
		minOffset:           minOffset,
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
		lastCommittedOffset := m.minOffset
		for {
			select {
			case <-time.After(resetInterval):
				offset := m.list.setToHighestContiguous()
				if lastCommittedOffset != offset {
					m.offsetCommitCount.Inc(1)
					m.lastCommittedOffset.Update(offset)
					m.markOffsetFunction(offset)
					lastCommittedOffset = offset
				}
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
