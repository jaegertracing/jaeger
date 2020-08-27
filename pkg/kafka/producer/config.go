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

	"github.com/Shopify/sarama"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

// Builder builds a new kafka producer
type Builder interface {
	NewProducer(logger *zap.Logger) (sarama.AsyncProducer, error)
}

// Configuration describes the configuration properties needed to create a Kafka producer
type Configuration struct {
	Brokers                   []string                `mapstructure:"brokers"`
	RequiredAcks              sarama.RequiredAcks     `mapstructure:"required_acks"`
	Compression               sarama.CompressionCodec `mapstructure:"compression"`
	CompressionLevel          int                     `mapstructure:"compression_level"`
	ProtocolVersion           string                  `mapstructure:"protocol_version"`
	BatchLinger               time.Duration           `mapstructure:"batch_linger"`
	BatchSize                 int                     `mapstructure:"batch_size"`
	BatchMinMessages          int                     `mapstructure:"batch_min_messages"`
	BatchMaxMessages          int                     `mapstructure:"batch_max_messages"`
	auth.AuthenticationConfig `mapstructure:"authentication"`
}

// NewProducer creates a new asynchronous kafka producer
func (c *Configuration) NewProducer(logger *zap.Logger) (sarama.AsyncProducer, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.RequiredAcks = c.RequiredAcks
	saramaConfig.Producer.Compression = c.Compression
	saramaConfig.Producer.CompressionLevel = c.CompressionLevel
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Flush.Bytes = c.BatchSize
	saramaConfig.Producer.Flush.Frequency = c.BatchLinger
	saramaConfig.Producer.Flush.Messages = c.BatchMinMessages
	saramaConfig.Producer.Flush.MaxMessages = c.BatchMaxMessages
	if len(c.ProtocolVersion) > 0 {
		ver, err := sarama.ParseKafkaVersion(c.ProtocolVersion)
		if err != nil {
			return nil, err
		}
		saramaConfig.Version = ver
	}
	if err := c.AuthenticationConfig.SetConfiguration(saramaConfig, logger); err != nil {
		return nil, err
	}
	return sarama.NewAsyncProducer(c.Brokers, saramaConfig)
}
