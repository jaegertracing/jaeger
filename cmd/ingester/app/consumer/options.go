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
	"flag"
	"strconv"
	"strings"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/kafka/config"
)

const (
	configPrefix      = "ingester-consumer"
	suffixBrokers     = ".brokers"
	suffixTopic       = ".topic"
	suffixGroupID     = ".group-id"
	suffixParallelism = ".parallelism"

	defaultBroker      = "127.0.0.1:9092"
	defaultTopic       = "jaeger-ingester-spans"
	defaultGroupID     = "jaeger-ingester"
	defaultParallelism = 1000
)

// Options stores the configuration options for a Kafka consumer
type Options struct {
	config.ConsumerConfiguration
	Parallelism int
}

// AddFlags adds flags for Options
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		configPrefix+suffixBrokers,
		defaultBroker,
		"The comma-separated list of kafka brokers. i.e. '127.0.0.1:9092,0.0.0:1234'")
	flagSet.String(
		configPrefix+suffixTopic,
		defaultTopic,
		"The name of the kafka topic to consume from")
	flagSet.String(
		configPrefix+suffixGroupID,
		defaultGroupID,
		"The Consumer Group that ingester will be consuming on behalf of")
	flagSet.String(
		configPrefix+suffixParallelism,
		strconv.Itoa(defaultParallelism),
		"The number of messages to process in parallel")
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.Brokers = strings.Split(v.GetString(configPrefix+suffixBrokers), ",")
	opt.Topic = v.GetString(configPrefix + suffixTopic)
	opt.GroupID = v.GetString(configPrefix + suffixGroupID)
	opt.Parallelism = v.GetInt(configPrefix + suffixParallelism)
}
