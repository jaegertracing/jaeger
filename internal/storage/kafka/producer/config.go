// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"time"

	"github.com/Shopify/sarama"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/kafka/auth"
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
	MaxMessageBytes           int                     `mapstructure:"max_message_bytes"`
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
	saramaConfig.Producer.MaxMessageBytes = c.MaxMessageBytes
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
