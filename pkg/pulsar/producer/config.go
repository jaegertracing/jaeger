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

package producer

import (
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	"go.uber.org/zap"
)

// Builder builds a new kafka producer
type Builder interface {
	NewProducer(topic string, logger *zap.Logger) (pulsar.Producer, error)
}

// Configuration describes the configuration properties needed to create a Kafka producer
type Configuration struct {
	URL                     string                  `mapstructure:"url"`
	Token                   string                  `mapstructure:"token"`
	Topic                   string                  `mapstructure:"topic"`
	Compression             pulsar.CompressionType  `mapstructure:"compression_type"`
	CompressionLevel        pulsar.CompressionLevel `mapstructure:"compression_level"`
	MaxPendingMessages      int                     `mapstructure:"max_pending_messages"`
	BatchingMaxSize         int                     `mapstructure:"batching_max_size"`
	BatchingMaxPublishDelay time.Duration           `mapstructure:"batching_max_publish_delay"`
	BatchingMaxMessages     int                     `mapstructure:"batching_max_messages"`
}

// NewProducer creates a new asynchronous kafka producer
func (c *Configuration) NewProducer(topic string, logger *zap.Logger) (pulsar.Producer, error) {
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

	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic:                   topic,
		CompressionType:         c.Compression,
		CompressionLevel:        c.CompressionLevel,
		MaxPendingMessages:      c.MaxPendingMessages,
		BatchingMaxPublishDelay: time.Duration(c.BatchingMaxPublishDelay),
		BatchingMaxMessages:     uint(c.BatchingMaxMessages),
		BatchingMaxSize:         uint(c.BatchingMaxSize),
	})
	return producer, err
}
