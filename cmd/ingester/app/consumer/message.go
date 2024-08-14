// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"github.com/Shopify/sarama"
)

// Message contains the parts of a sarama ConsumerMessage that we care about.
type Message interface {
	Key() []byte
	Value() []byte
	Topic() string
	Partition() int32
	Offset() int64
}

type saramaMessageWrapper struct {
	*sarama.ConsumerMessage
}

func (m saramaMessageWrapper) Key() []byte {
	return m.ConsumerMessage.Key
}

func (m saramaMessageWrapper) Value() []byte {
	return m.ConsumerMessage.Value
}

func (m saramaMessageWrapper) Topic() string {
	return m.ConsumerMessage.Topic
}

func (m saramaMessageWrapper) Partition() int32 {
	return m.ConsumerMessage.Partition
}

func (m saramaMessageWrapper) Offset() int64 {
	return m.ConsumerMessage.Offset
}
