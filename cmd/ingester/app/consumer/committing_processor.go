// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"errors"
	"io"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
)

type comittingProcessor struct {
	processor processor.SpanProcessor
	marker    offsetMarker
	io.Closer
}

type offsetMarker interface {
	MarkOffset(int64)
}

// NewCommittingProcessor returns a processor that commits message offsets to Kafka
func NewCommittingProcessor(processor processor.SpanProcessor, marker offsetMarker) processor.SpanProcessor {
	return &comittingProcessor{
		processor: processor,
		marker:    marker,
	}
}

func (d *comittingProcessor) Process(message processor.Message) error {
	if msg, ok := message.(Message); ok {
		err := d.processor.Process(message)
		if err == nil {
			d.marker.MarkOffset(msg.Offset())
		}
		return err
	}
	return errors.New("committing processor used with non-kafka message")
}
