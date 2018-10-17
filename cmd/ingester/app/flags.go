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
	"strings"

	"github.com/spf13/viper"

	kafkaConsumer "github.com/jaegertracing/jaeger/pkg/kafka/consumer"
)

const (
	// EncodingJSON indicates spans are encoded as a json byte array
	EncodingJSON = "json"
	// EncodingProto indicates spans are encoded as a protobuf byte array
	EncodingProto = "protobuf"

	// ConfigPrefix is a prefix fro the ingester flags
	ConfigPrefix = "ingester"
	// SuffixBrokers is a suffix for the brokers flag
	SuffixBrokers = ".brokers"
	// SuffixTopic is a suffix for the topic flag
	SuffixTopic = ".topic"
	// SuffixGroupID is a suffix for the group-id flag
	SuffixGroupID = ".group-id"
	// SuffixParallelism is a suffix for the parallelism flag
	SuffixParallelism = ".parallelism"
	// SuffixEncoding is a suffix for the encoding flag
	SuffixEncoding = ".encoding"
	// SuffixMaxReadsPerSecond is a suffix for the max-reads-per-second flag
	SuffixMaxReadsPerSecond = ".max-reads-per-second"
	// SuffixMaxBurstReadsPerSecond is a suffix for the max-burst-reads-per-second flag
	SuffixMaxBurstReadsPerSecond = ".max-burst-reads-per-second"

	// DefaultBroker is the default kafka broker
	DefaultBroker = "127.0.0.1:9092"
	// DefaultTopic is the default kafka topic
	DefaultTopic = "jaeger-spans"
	// DefaultGroupID is the default consumer Group ID
	DefaultGroupID = "jaeger-ingester"
	// DefaultParallelism is the default parallelism for the span processor
	DefaultParallelism = 1000
	// DefaultEncoding is the default span encoding
	DefaultEncoding = EncodingProto
	// DefaultMaxReadsPerSecond is the default max reads per second for the span processor
	DefaultMaxReadsPerSecond = -1.0
	// DefaultMaxBurstReadsPerSecond is the default max burst reads per second for the span processor
	DefaultMaxBurstReadsPerSecond = 1.0
)

// Options stores the configuration options for the Ingester
type Options struct {
	kafkaConsumer.Configuration
	Parallelism            int
	Encoding               string
	MaxReadsPerSecond      float64
	MaxBurstReadsPerSecond float64
}

// AddFlags adds flags for Builder
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		ConfigPrefix+SuffixBrokers,
		DefaultBroker,
		"The comma-separated list of kafka brokers. i.e. '127.0.0.1:9092,0.0.0:1234'")
	flagSet.String(
		ConfigPrefix+SuffixTopic,
		DefaultTopic,
		"The name of the kafka topic to consume from")
	flagSet.String(
		ConfigPrefix+SuffixGroupID,
		DefaultGroupID,
		"The Consumer Group that ingester will be consuming on behalf of")
	flagSet.Int(
		ConfigPrefix+SuffixParallelism,
		DefaultParallelism,
		"The number of messages to process in parallel")
	flagSet.String(
		ConfigPrefix+SuffixEncoding,
		DefaultEncoding,
		fmt.Sprintf(`The encoding of spans ("%s" or "%s") consumed from kafka`, EncodingProto, EncodingJSON))
	flagSet.Float64(
		ConfigPrefix+SuffixMaxReadsPerSecond,
		DefaultMaxReadsPerSecond,
		"The number of reads per second")
	flagSet.Float64(
		ConfigPrefix+SuffixMaxBurstReadsPerSecond,
		DefaultMaxBurstReadsPerSecond,
		"The number of burst reads per second")
}

// InitFromViper initializes Builder with properties from viper
func (o *Options) InitFromViper(v *viper.Viper) {
	o.Brokers = strings.Split(v.GetString(ConfigPrefix+SuffixBrokers), ",")
	o.Topic = v.GetString(ConfigPrefix + SuffixTopic)
	o.GroupID = v.GetString(ConfigPrefix + SuffixGroupID)
	o.Parallelism = v.GetInt(ConfigPrefix + SuffixParallelism)
	o.Encoding = v.GetString(ConfigPrefix + SuffixEncoding)
	o.MaxReadsPerSecond = v.GetFloat64(ConfigPrefix + SuffixMaxReadsPerSecond)
	o.MaxBurstReadsPerSecond = v.GetFloat64(ConfigPrefix + SuffixMaxBurstReadsPerSecond)
}
