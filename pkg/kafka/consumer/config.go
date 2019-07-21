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
	"io"

	"github.com/bsm/sarama-cluster"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

// Consumer is an interface to features of Sarama that are necessary for the consumer
type Consumer interface {
	Partitions() <-chan cluster.PartitionConsumer
	MarkPartitionOffset(topic string, partition int32, offset int64, metadata string)
	io.Closer
}

// Builder builds a new kafka consumer
type Builder interface {
	NewConsumer() (Consumer, error)
}

// Configuration describes the configuration properties needed to create a Kafka consumer
type Configuration struct {
	Brokers  []string
	Topic    string
	GroupID  string
	ClientID string
	Consumer
	auth.AuthenticationConfig
}

// NewConsumer creates a new kafka consumer
func (c *Configuration) NewConsumer() (Consumer, error) {
	saramaConfig := cluster.NewConfig()
	saramaConfig.Group.Mode = cluster.ConsumerModePartitions
	saramaConfig.ClientID = c.ClientID
	c.AuthenticationConfig.SetConfiguration(&saramaConfig.Config)
	return cluster.NewConsumer(c.Brokers, c.GroupID, []string{c.Topic}, saramaConfig)
}
