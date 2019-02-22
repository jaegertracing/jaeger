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
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/kafka/producer"
)

const (
	// EncodingJSON is used for spans encoded as Protobuf-based JSON.
	EncodingJSON = "json"
	// EncodingProto is used for spans encoded as Protobuf.
	EncodingProto = "protobuf"
	// EncodingZipkinThrift is used for spans encoded as Zipkin Thrift.
	EncodingZipkinThrift = "zipkin-thrift"

	configPrefix     = "kafka.producer"
	deprecatedPrefix = "kafka"
	suffixBrokers    = ".brokers"
	suffixTopic      = ".topic"
	suffixEncoding   = ".encoding"

	defaultBroker   = "127.0.0.1:9092"
	defaultTopic    = "jaeger-spans"
	defaultEncoding = EncodingProto
)

var (
	// AllEncodings is a list of all supported encodings.
	AllEncodings = []string{EncodingJSON, EncodingProto, EncodingZipkinThrift}
)

// Options stores the configuration options for Kafka
type Options struct {
	config   producer.Configuration
	topic    string
	encoding string
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		configPrefix+suffixBrokers,
		defaultBroker,
		"(experimental) The comma-separated list of kafka brokers. i.e. '127.0.0.1:9092,0.0.0:1234'")
	flagSet.String(
		configPrefix+suffixTopic,
		defaultTopic,
		"(experimental) The name of the kafka topic")
	flagSet.String(
		configPrefix+suffixEncoding,
		defaultEncoding,
		fmt.Sprintf(`(experimental) Encoding of spans ("%s" or "%s") sent to kafka.`, EncodingJSON, EncodingProto),
	)

	// TODO: Remove deprecated flags after 1.11
	flagSet.String(
		deprecatedPrefix+suffixBrokers,
		"",
		fmt.Sprintf("Deprecated; replaced by %s", configPrefix+suffixBrokers))
	flagSet.String(
		deprecatedPrefix+suffixTopic,
		"",
		fmt.Sprintf("Deprecated; replaced by %s", configPrefix+suffixTopic))
	flagSet.String(
		deprecatedPrefix+suffixEncoding,
		"",
		fmt.Sprintf("Deprecated; replaced by %s", configPrefix+suffixEncoding))
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.config = producer.Configuration{
		Brokers: strings.Split(stripWhiteSpace(v.GetString(configPrefix+suffixBrokers)), ","),
	}
	opt.topic = v.GetString(configPrefix + suffixTopic)
	opt.encoding = v.GetString(configPrefix + suffixEncoding)

	if brokers := v.GetString(deprecatedPrefix + suffixBrokers); brokers != "" {
		fmt.Printf("WARNING: found deprecated option %s, please use %s instead\n",
			deprecatedPrefix+suffixBrokers,
			configPrefix+suffixBrokers,
		)
		opt.config = producer.Configuration{
			Brokers: strings.Split(stripWhiteSpace(brokers), ","),
		}
	}
	if topic := v.GetString(deprecatedPrefix + suffixTopic); topic != "" {
		fmt.Printf("WARNING: found deprecated option %s, please use %s instead\n",
			deprecatedPrefix+suffixTopic,
			configPrefix+suffixTopic,
		)
		opt.topic = topic
	}
	if encoding := v.GetString(deprecatedPrefix + suffixEncoding); encoding != "" {
		fmt.Printf("WARNING: found deprecated option %s, please use %s instead\n",
			deprecatedPrefix+suffixEncoding,
			configPrefix+suffixEncoding,
		)
		opt.encoding = encoding
	}
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.Replace(str, " ", "", -1)
}
