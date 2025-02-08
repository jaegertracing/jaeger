// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/internal/storage/v1/kafka"
	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
	kafkaConsumer "github.com/jaegertracing/jaeger/pkg/kafka/consumer"
)

const (
	// ConfigPrefix is a prefix for the ingester flags
	ConfigPrefix = "ingester"
	// KafkaConsumerConfigPrefix is a prefix for the Kafka flags
	KafkaConsumerConfigPrefix = "kafka.consumer"
	// SuffixBrokers is a suffix for the brokers flag
	SuffixBrokers = ".brokers"
	// SuffixTopic is a suffix for the topic flag
	SuffixTopic = ".topic"
	// SuffixRackID is a suffix for the consumer rack-id flag
	SuffixRackID = ".rack-id"
	// SuffixFetchMaxMessageBytes is a suffix for the consumer fetch-max-message-bytes flag
	SuffixFetchMaxMessageBytes = ".fetch-max-message-bytes"
	// SuffixGroupID is a suffix for the group-id flag
	SuffixGroupID = ".group-id"
	// SuffixClientID is a suffix for the client-id flag
	SuffixClientID = ".client-id"
	// SuffixProtocolVersion Kafka protocol version - must be supported by kafka server
	SuffixProtocolVersion = ".protocol-version"
	// SuffixEncoding is a suffix for the encoding flag
	SuffixEncoding = ".encoding"
	// SuffixDeadlockInterval is a suffix for deadlock detecor flag
	SuffixDeadlockInterval = ".deadlockInterval"
	// SuffixParallelism is a suffix for the parallelism flag
	SuffixParallelism = ".parallelism"
	// SuffixHTTPPort is a suffix for the HTTP port
	SuffixHTTPPort = ".http-port"
	// DefaultBroker is the default kafka broker
	DefaultBroker = "127.0.0.1:9092"
	// DefaultTopic is the default kafka topic
	DefaultTopic = "jaeger-spans"
	// DefaultGroupID is the default consumer Group ID
	DefaultGroupID = "jaeger-ingester"
	// DefaultClientID is the default consumer Client ID
	DefaultClientID = "jaeger-ingester"
	// DefaultParallelism is the default parallelism for the span processor
	DefaultParallelism = 1000
	// DefaultEncoding is the default span encoding
	DefaultEncoding = kafka.EncodingProto
	// DefaultDeadlockInterval is the default deadlock interval
	DefaultDeadlockInterval = time.Duration(0)
	// DefaultFetchMaxMessageBytes is the default for kafka.consumer.fetch-max-message-bytes flag
	DefaultFetchMaxMessageBytes = 1024 * 1024 // 1MB
)

// Options stores the configuration options for the Ingester
type Options struct {
	kafkaConsumer.Configuration `mapstructure:",squash"`
	Parallelism                 int           `mapstructure:"parallelism"`
	Encoding                    string        `mapstructure:"encoding"`
	DeadlockInterval            time.Duration `mapstructure:"deadlock_interval"`
}

// AddFlags adds flags for Builder
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		ConfigPrefix+SuffixParallelism,
		strconv.Itoa(DefaultParallelism),
		"The number of messages to process in parallel")
	flagSet.Duration(
		ConfigPrefix+SuffixDeadlockInterval,
		DefaultDeadlockInterval,
		"Interval to check for deadlocks. If no messages gets processed in given time, ingester app will exit. Value of 0 disables deadlock check.")

	// Authentication flags
	flagSet.String(
		KafkaConsumerConfigPrefix+SuffixBrokers,
		DefaultBroker,
		"The comma-separated list of kafka brokers. i.e. '127.0.0.1:9092,0.0.0:1234'")
	flagSet.String(
		KafkaConsumerConfigPrefix+SuffixTopic,
		DefaultTopic,
		"The name of the kafka topic to consume from")
	flagSet.String(
		KafkaConsumerConfigPrefix+SuffixGroupID,
		DefaultGroupID,
		"The Consumer Group that ingester will be consuming on behalf of")
	flagSet.String(
		KafkaConsumerConfigPrefix+SuffixClientID,
		DefaultClientID,
		"The Consumer Client ID that ingester will use")
	flagSet.String(
		KafkaConsumerConfigPrefix+SuffixProtocolVersion,
		"",
		"Kafka protocol version - must be supported by kafka server")
	flagSet.String(
		KafkaConsumerConfigPrefix+SuffixEncoding,
		DefaultEncoding,
		fmt.Sprintf(`The encoding of spans ("%s") consumed from kafka`, strings.Join(kafka.AllEncodings, "\", \"")))
	flagSet.String(
		KafkaConsumerConfigPrefix+SuffixRackID,
		"",
		"Rack identifier for this client. This can be any string value which indicates where this client is located. It corresponds with the broker config `broker.rack`")
	flagSet.Int(
		KafkaConsumerConfigPrefix+SuffixFetchMaxMessageBytes,
		DefaultFetchMaxMessageBytes,
		"The maximum number of message bytes to fetch from the broker in a single request. So you must be sure this is at least as large as your largest message.")

	auth.AddFlags(KafkaConsumerConfigPrefix, flagSet)
}

// InitFromViper initializes Builder with properties from viper
func (o *Options) InitFromViper(v *viper.Viper) {
	o.Brokers = strings.Split(stripWhiteSpace(v.GetString(KafkaConsumerConfigPrefix+SuffixBrokers)), ",")
	o.Topic = v.GetString(KafkaConsumerConfigPrefix + SuffixTopic)
	o.GroupID = v.GetString(KafkaConsumerConfigPrefix + SuffixGroupID)
	o.ClientID = v.GetString(KafkaConsumerConfigPrefix + SuffixClientID)
	o.ProtocolVersion = v.GetString(KafkaConsumerConfigPrefix + SuffixProtocolVersion)
	o.Encoding = v.GetString(KafkaConsumerConfigPrefix + SuffixEncoding)
	o.RackID = v.GetString(KafkaConsumerConfigPrefix + SuffixRackID)
	o.FetchMaxMessageBytes = v.GetInt32(KafkaConsumerConfigPrefix + SuffixFetchMaxMessageBytes)

	o.Parallelism = v.GetInt(ConfigPrefix + SuffixParallelism)
	o.DeadlockInterval = v.GetDuration(ConfigPrefix + SuffixDeadlockInterval)
	authenticationOptions := auth.AuthenticationConfig{}
	authenticationOptions.InitFromViper(KafkaConsumerConfigPrefix, v)
	o.AuthenticationConfig = authenticationOptions
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.ReplaceAll(str, " ", "")
}
