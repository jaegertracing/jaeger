// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stretchr/testify/assert"
)

func TestSaramaMessageWrapper(t *testing.T) {
	saramaMessage := &sarama.ConsumerMessage{
		Key:       []byte("some key"),
		Value:     []byte("some value"),
		Topic:     "some topic",
		Partition: 555,
		Offset:    1942,
	}

	wrappedMessage := saramaMessageWrapper{saramaMessage}

	assert.Equal(t, saramaMessage.Key, wrappedMessage.Key())
	assert.Equal(t, saramaMessage.Value, wrappedMessage.Value())
	assert.Equal(t, saramaMessage.Topic, wrappedMessage.Topic())
	assert.Equal(t, saramaMessage.Partition, wrappedMessage.Partition())
	assert.Equal(t, saramaMessage.Offset, wrappedMessage.Offset())
}
