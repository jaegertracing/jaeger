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

package app

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
	kafkaConsumer "github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
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
	AddOTELFlags(flagSet)
}

// AddOTELFlags adds only OTEL flags
func AddOTELFlags(flagSet *flag.FlagSet) {
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

	o.Parallelism = v.GetInt(ConfigPrefix + SuffixParallelism)
	o.DeadlockInterval = v.GetDuration(ConfigPrefix + SuffixDeadlockInterval)
	authenticationOptions := auth.AuthenticationConfig{}
	authenticationOptions.InitFromViper(KafkaConsumerConfigPrefix, v)
	o.AuthenticationConfig = authenticationOptions
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.Replace(str, " ", "", -1)
}
