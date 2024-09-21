// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"context"
	"time"

	"github.com/Shopify/sarama"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

// Builder builds a new kafka producer
type Builder interface {
	NewProducer(ctx context.Context) (sarama.AsyncProducer, error)
}

// Configuration describes the configuration properties needed to create a Kafka producer
type Configuration struct {
	Authentication  auth.AuthenticationConfig `mapstructure:"auth"`
	Batch           Batch                     `mapstructure:"batch"`
	Broker          Broker                    `mapstructure:"broker"`
	Compression     Compression               `mapstructure:"compression"`
	MaxMessageBytes int                       `mapstructure:"max_message_bytes"`
	ProtocolVersion string                    `mapstructure:"protocol_version"`
}

type Batch struct {
	// Linger is the time interval to wait before sending records to the Kafka broker.
	// A higher value will reduce the number requests to Kafka but increase latency and the possibility
	// of data loss in case of process restart (see https://kafka.apache.org/documentation/").
	Linger time.Duration `mapstructure:"linger"`
	// MaxMessages is the maximum number of message to batch before sending records to Kafka.
	MaxMessages int `mapstructure:"max_messages"`
	// MinMessages is the best-effort number of messages needed to send a batch of records to Kafka.
	// A higher value will reduce the number requests to Kafka but increase latency and the possibility
	// of data loss in case of process restart (see https://kafka.apache.org/documentation/").
	MinMessages int `mapstructure:"min_messages"`
	// Size is the best-effort number of bytes needed to send a batch of records to Kafka.
	// A higher value will reduce the number requests to Kafka but increase latency and the possibility
	// of data loss in case of process restart (see https://kafka.apache.org/documentation/").
	Size int `mapstructure:"size"`
}

type Broker struct {
	// Addresses contains a list of the broker addresses.
	Addresses []string `mapstructure:"addresses"`
	// RequiredAcks tells the level of acknowledgement reliability needed from the
	// broker when producing requests.
	RequiredAcks sarama.RequiredAcks `mapstructure:"required_acks"`
}

type Compression struct {
	// Level contains the level of compression to use for messages.
	Level int `mapstructure:"level"`
	// Type contains the type of compression to use for messages.
	Type sarama.CompressionCodec `mapstructure:"type"`
}

// NewProducer creates a new asynchronous kafka producer
func (c *Configuration) NewProducer(ctx context.Context) (sarama.AsyncProducer, error) {
	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.RequiredAcks = c.Broker.RequiredAcks
	saramaConfig.Producer.Compression = c.Compression.Type
	saramaConfig.Producer.CompressionLevel = c.Compression.Level
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Flush.Bytes = c.Batch.Size
	saramaConfig.Producer.Flush.Frequency = c.Batch.Linger
	saramaConfig.Producer.Flush.Messages = c.Batch.MinMessages
	saramaConfig.Producer.Flush.MaxMessages = c.Batch.MaxMessages
	saramaConfig.Producer.MaxMessageBytes = c.MaxMessageBytes
	if len(c.ProtocolVersion) > 0 {
		ver, err := sarama.ParseKafkaVersion(c.ProtocolVersion)
		if err != nil {
			return nil, err
		}
		saramaConfig.Version = ver
	}
	if err := c.Authentication.SetConfiguration(ctx, saramaConfig); err != nil {
		return nil, err
	}
	return sarama.NewAsyncProducer(c.Broker.Addresses, saramaConfig)
}
