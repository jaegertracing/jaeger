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
