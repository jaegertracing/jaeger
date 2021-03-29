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
	"errors"
	"io"

	"github.com/jaegertracing/jaeger/cmd/ingester/app/processor"
)

type committingProcessor struct {
	processor processor.SpanProcessor
	marker    offsetMarker
	io.Closer
}

type offsetMarker interface {
	MarkOffset(int64)
}

// NewCommittingProcessor returns a processor that commits message offsets to Kafka
func NewCommittingProcessor(processor processor.SpanProcessor, marker offsetMarker) processor.SpanProcessor {
	return &committingProcessor{
		processor: processor,
		marker:    marker,
	}
}

func (d *committingProcessor) Process(message processor.Message) error {
	if msg, ok := message.(Message); ok {
		err := d.processor.Process(message)
		if err == nil {
			d.marker.MarkOffset(msg.Offset())
		}
		return err
	}
	return errors.New("committing processor used with non-kafka message")
}
