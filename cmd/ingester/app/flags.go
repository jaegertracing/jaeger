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

	kafkaConsumer "github.com/jaegertracing/jaeger/pkg/kafka/consumer"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

const (
	// ConfigPrefix is a prefix for the ingester flags
	ConfigPrefix = "ingester"
	// KafkaConsumerConfigPrefix is a prefix for the Kafka flags
	KafkaConsumerConfigPrefix = "kafka.consumer"
	// DeprecatedKafkaConfigPrefix is a prefix for the Kafka flags that is replaced by KafkaConfigPrefix
	DeprecatedKafkaConfigPrefix = "kafka"
	// SuffixBrokers is a suffix for the brokers flag
	SuffixBrokers = ".brokers"
	// SuffixTopic is a suffix for the topic flag
	SuffixTopic = ".topic"
	// SuffixGroupID is a suffix for the group-id flag
	SuffixGroupID = ".group-id"
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
	// DefaultParallelism is the default parallelism for the span processor
	DefaultParallelism = 1000
	// DefaultEncoding is the default span encoding
	DefaultEncoding = kafka.EncodingProto
	// DefaultDeadlockInterval is the default deadlock interval
	DefaultDeadlockInterval = 1 * time.Minute
	// DefaultHTTPPort is the default HTTP port (e.g. for /metrics)
	DefaultHTTPPort = 14271
	// IngesterDefaultHealthCheckHTTPPort is the default HTTP Port for health check
	IngesterDefaultHealthCheckHTTPPort = 14270
)

// Options stores the configuration options for the Ingester
type Options struct {
	kafkaConsumer.Configuration
	Parallelism int
	Encoding    string
	// IngesterHTTPPort is the port that the ingester service listens in on for http requests
	IngesterHTTPPort int
	DeadlockInterval time.Duration
}

// AddFlags adds flags for Builder
func AddFlags(flagSet *flag.FlagSet) {
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
		KafkaConsumerConfigPrefix+SuffixEncoding,
		DefaultEncoding,
		fmt.Sprintf(`The encoding of spans ("%s") consumed from kafka`, strings.Join(kafka.AllEncodings, "\", \"")))
	flagSet.String(
		ConfigPrefix+SuffixParallelism,
		strconv.Itoa(DefaultParallelism),
		"The number of messages to process in parallel")
	flagSet.Int(
		ConfigPrefix+SuffixHTTPPort,
		DefaultHTTPPort,
		"The http port for the ingester service")
	flagSet.Duration(
		ConfigPrefix+SuffixDeadlockInterval,
		DefaultDeadlockInterval,
		"Interval to check for deadlocks. If no messages gets processed in given time, ingester app will exit. Value of 0 disables deadlock check.")

	// TODO: Remove deprecated flags after 1.11
	flagSet.String(
		DeprecatedKafkaConfigPrefix+SuffixBrokers,
		"",
		fmt.Sprintf("Deprecated; replaced by %s", KafkaConsumerConfigPrefix+SuffixBrokers))
	flagSet.String(
		DeprecatedKafkaConfigPrefix+SuffixTopic,
		"",
		fmt.Sprintf("Deprecated; replaced by %s", KafkaConsumerConfigPrefix+SuffixTopic))
	flagSet.String(
		DeprecatedKafkaConfigPrefix+SuffixGroupID,
		"",
		fmt.Sprintf("Deprecated; replaced by %s", KafkaConsumerConfigPrefix+SuffixGroupID))
	flagSet.String(
		DeprecatedKafkaConfigPrefix+SuffixEncoding,
		"",
		fmt.Sprintf("Deprecated; replaced by %s", KafkaConsumerConfigPrefix+SuffixEncoding))
}

// InitFromViper initializes Builder with properties from viper
func (o *Options) InitFromViper(v *viper.Viper) {
	o.Brokers = strings.Split(v.GetString(KafkaConsumerConfigPrefix+SuffixBrokers), ",")
	o.Topic = v.GetString(KafkaConsumerConfigPrefix + SuffixTopic)
	o.GroupID = v.GetString(KafkaConsumerConfigPrefix + SuffixGroupID)
	o.Encoding = v.GetString(KafkaConsumerConfigPrefix + SuffixEncoding)

	if brokers := v.GetString(DeprecatedKafkaConfigPrefix+SuffixBrokers); brokers != "" {
		fmt.Printf("WARNING: found deprecated option %s, please use %s instead\n",
			DeprecatedKafkaConfigPrefix+SuffixBrokers,
			KafkaConsumerConfigPrefix+SuffixBrokers,
		)
		o.Brokers = strings.Split(brokers, ",")
	}
	if topic := v.GetString(DeprecatedKafkaConfigPrefix + SuffixTopic); topic != "" {
		fmt.Printf("WARNING: found deprecated option %s, please use %s instead\n",
			DeprecatedKafkaConfigPrefix+SuffixTopic,
			KafkaConsumerConfigPrefix+SuffixTopic,
		)
		o.Topic = topic
	}
	if groupID := v.GetString(DeprecatedKafkaConfigPrefix + SuffixGroupID); groupID != "" {
		fmt.Printf("WARNING: found deprecated option %s, please use %s instead\n",
			DeprecatedKafkaConfigPrefix+SuffixGroupID,
			KafkaConsumerConfigPrefix+SuffixGroupID,
		)
		o.GroupID = groupID
	}
	if encoding := v.GetString(DeprecatedKafkaConfigPrefix + SuffixEncoding); encoding != "" {
		fmt.Printf("WARNING: found deprecated option %s, please use %s instead\n",
			DeprecatedKafkaConfigPrefix+SuffixEncoding,
			KafkaConsumerConfigPrefix+SuffixEncoding,
		)
		o.Encoding = encoding
	}

	o.Parallelism = v.GetInt(ConfigPrefix + SuffixParallelism)
	o.IngesterHTTPPort = v.GetInt(ConfigPrefix + SuffixHTTPPort)

	o.DeadlockInterval = v.GetDuration(ConfigPrefix + SuffixDeadlockInterval)
}
