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
