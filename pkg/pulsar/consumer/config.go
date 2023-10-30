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
	"strings"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"go.uber.org/zap"
)

// Consumer is an interface to features of Sarama that are necessary for the consumer
// type Consumer interface {
// 	Partitions() <-chan cluster.PartitionConsumer
// 	MarkPartitionOffset(topic string, partition int32, offset int64, metadata string)
// 	io.Closer
// }

// Builder builds a new kafka consumer
type Builder interface {
	NewConsumer() (pulsar.Consumer, error)
}

// Configuration describes the configuration properties needed to create a Kafka consumer
type Configuration struct {
	URL               string `mapstructure:"url"`
	Topic             string `mapstructure:"topic"`
	Token             string `mapstructure:"token"`
	SubscriptionName  string `mapstructure:"subscription_name"`
	SubscriptionType  string `mapstructure:"subscription_type"`
	ReceiverQueueSize int    `mapstructure:"receiver_queue_size"`

	consumer pulsar.Consumer
}

// NewConsumer creates a new kafka consumer
func (c *Configuration) NewConsumer(logger *zap.Logger) (pulsar.Consumer, error) {
	options := pulsar.ClientOptions{
		URL:               c.URL,
		ConnectionTimeout: 5 * time.Second,
	}
	if c.Token != "" {
		options.Authentication = pulsar.NewAuthenticationToken(c.Token)
	}

	client, err := pulsar.NewClient(options)
	if err != nil {
		return nil, err
	}

	var stype pulsar.SubscriptionType

	switch strings.ToLower(c.SubscriptionType) {
	case "exclusive":
		stype = pulsar.Exclusive
	case "shared":
		stype = pulsar.Shared
	case "failover":
		stype = pulsar.Failover
	default:
		return nil, errors.New("invalid pulsar subscription type.")
	}

	copts := pulsar.ConsumerOptions{
		Topic:             c.Topic,
		SubscriptionName:  c.SubscriptionName,
		Type:              stype,
		ReceiverQueueSize: c.ReceiverQueueSize,
	}

	consumer, err := client.Subscribe(copts)
	return consumer, err
}
