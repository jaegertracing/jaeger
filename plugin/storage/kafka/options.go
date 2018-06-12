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

package kafka

import (
	"flag"
	"strings"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/kafka/config"
)

const (
	configPrefix  = "kafka"
	suffixBrokers = ".brokers"
	suffixTopic   = ".topic"
)

// Options stores the configuration options for Kafka
type Options struct {
	config config.ProducerBuilder
	topic  string
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		configPrefix+suffixBrokers,
		"127.0.0.1:9092",
		"The comma-separated list of kafka brokers. i.e. '127.0.0.1:9092,0.0.0:1234'")
	flagSet.String(
		configPrefix+suffixTopic,
		"jaeger-spans",
		"The name of the kafka topic")
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.config = &config.Configuration{
		Brokers: strings.Split(v.GetString(configPrefix+suffixBrokers), ","),
	}
	opt.topic = v.GetString(configPrefix + suffixTopic)
}
