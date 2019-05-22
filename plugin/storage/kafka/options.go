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

	"github.com/Shopify/sarama"
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

	configPrefix           = "kafka.producer"
	suffixBrokers          = ".brokers"
	suffixTopic            = ".topic"
	suffixEncoding         = ".encoding"
	suffixRequiredAcks     = ".required.acks"
	suffixCompression      = ".compression"
	suffixCompressionLevel = ".compression.level"

	defaultBroker           = "127.0.0.1:9092"
	defaultTopic            = "jaeger-spans"
	defaultEncoding         = EncodingProto
	defaultRequiredAcks     = 1
	defaultComression       = 0
	defaultCompressionLevel = -1000
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
	flagSet.Int(
		configPrefix+suffixRequiredAcks,
		defaultRequiredAcks,
		"(experimental) Required kafka broker acknowledgement. default = 1, no response = 0, wait for local = 1, wait for all = -1",
	)
	flagSet.Int(
		configPrefix+suffixCompression,
		defaultComression,
		"(experimental) Type of compression to use on messages. default = 0, none = 0, gzip = 1, snappy = 2, lz4 = 3, zstd = 4",
	)
	flagSet.Int(
		configPrefix+suffixCompressionLevel,
		defaultCompressionLevel,
		"(experimental) Level of compression to use on messages. default= -1000, gzip = 1-9 (default = 6), snappy = none, lz4 = 1-17 (default = 9), zstd = -131072 - 22 (default = 3)",
	)
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.config = producer.Configuration{
		Brokers:          strings.Split(stripWhiteSpace(v.GetString(configPrefix+suffixBrokers)), ","),
		RequiredAcks:     v.GetInt(configPrefix + suffixRequiredAcks),
		Compression:      v.GetInt(configPrefix + suffixCompression),
		CompressionLevel: getCompressionLevel(v.GetInt(configPrefix+suffixCompression), v.GetInt(configPrefix+suffixCompressionLevel)),
	}
	opt.topic = v.GetString(configPrefix + suffixTopic)
	opt.encoding = v.GetString(configPrefix + suffixEncoding)
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.Replace(str, " ", "", -1)
}

// getCompressionLevel return compression level for GZIP and ZSTD compression. rest compression will get default compression level (-1000)
func getCompressionLevel(compression, compressionLevel int) int {
	if compressionLevel == sarama.CompressionLevelDefault {
		switch sarama.CompressionCodec(compression) {
		case sarama.CompressionCodec(sarama.CompressionGZIP):
			return 6
		case sarama.CompressionCodec(sarama.CompressionZSTD):
			return 3
		}
	}

	return sarama.CompressionLevelDefault
}
