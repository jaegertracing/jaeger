package queue

import (
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

// NonQueue sends stuff directly without queueing. Useful for testing purposes
type NonQueue struct {
	processor func(*jaeger.Batch) error
}

// NewNonQueue returns direct processing "queue"
func NewNonQueue(processor func(*jaeger.Batch) error) *NonQueue {
	return &NonQueue{processor}
}

// Enqueue calls processor instead of queueing
func (n *NonQueue) Enqueue(batch *jaeger.Batch) error {
	return n.processor(batch)
}
