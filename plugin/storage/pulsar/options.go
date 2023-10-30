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

package pulsar

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/pulsar/producer"
)

const (
	// EncodingJSON is used for spans encoded as Protobuf-based JSON.
	EncodingJSON = "json"
	// EncodingProto is used for spans encoded as Protobuf.
	EncodingProto = "protobuf"
	// EncodingZipkinThrift is used for spans encoded as Zipkin Thrift.
	EncodingZipkinThrift = "zipkin-thrift"

	configPrefix           = "pulsar.producer"
	suffixURL              = ".url"
	suffixToken            = ".token"
	suffixTopic            = ".topic"
	suffixEncoding         = ".encoding"
	suffixCompression      = ".compression"
	suffixCompressionLevel = ".compression-level"
	suffixBatchLinger      = ".batch-linger"
	suffixBatchSize        = ".batch-size"
	suffixBatchMaxMessages = ".batch-max-messages"

	defaultURL                 = "pulsar://127.0.0.1:6650"
	defaultTopic               = "jaeger-spans"
	defaultEncoding            = EncodingProto
	defaultCompression         = "none"
	defaultCompressionLevel    = 0
	defaultBatchLinger         = 0
	defaultBatchSize           = 0
	defaultBatchingMaxMessages = 0
)

var (
	// AllEncodings is a list of all supported encodings.
	AllEncodings = []string{EncodingJSON, EncodingProto, EncodingZipkinThrift}

	// compressionModes is a mapping of supported CompressionType to compressionCodec along with default-0, faster-1, better-2 compression level
	compressionModes = map[string]struct {
		compressor              pulsar.CompressionType
		defaultCompressionLevel pulsar.CompressionLevel
		minCompressionLevel     pulsar.CompressionLevel
		maxCompressionLevel     pulsar.CompressionLevel
	}{
		"none": {
			compressor:              pulsar.NoCompression,
			defaultCompressionLevel: pulsar.Default,
			minCompressionLevel:     0,
			maxCompressionLevel:     2,
		},
		"zlib": {
			compressor:              pulsar.ZLib,
			defaultCompressionLevel: pulsar.Default,
			minCompressionLevel:     0,
			maxCompressionLevel:     2,
		},
		"lz4": {
			compressor:              pulsar.LZ4,
			defaultCompressionLevel: pulsar.Default,
			minCompressionLevel:     0,
			maxCompressionLevel:     2,
		},
		"zstd": {
			compressor:              pulsar.ZSTD,
			defaultCompressionLevel: pulsar.Default,
			minCompressionLevel:     0,
			maxCompressionLevel:     2,
		},
	}
)

// Options stores the configuration options for Pulsar
type Options struct {
	Config   producer.Configuration `mapstructure:",squash"`
	Encoding string                 `mapstructure:"encoding"`
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		configPrefix+suffixCompression,
		defaultCompression,
		"(experimental) Type of compression (none, zlib, lz4, zstd) to use on messages",
	)
	flagSet.Int(
		configPrefix+suffixCompressionLevel,
		defaultCompressionLevel,
		"(experimental) compression level to use on messages. zlib = (0-default, 1-faster, 2-better), lz4 = (0-default, 1-faster, 2-better), zstd = (0-default, 1-faster, 2-better)",
	)
	flagSet.Duration(
		configPrefix+suffixBatchLinger,
		defaultBatchLinger,
		"(experimental) Time interval to wait before sending records to pulsar. Higher value reduce request to pulsar but increase latency and the possibility of data loss in case of process restart.",
	)
	flagSet.Int(
		configPrefix+suffixBatchSize,
		defaultBatchSize,
		"(experimental) Number of bytes to batch before sending records to Pulsar. Higher value reduce request to pulsar but increase latency and the possibility of data loss in case of process restart.",
	)
	flagSet.Int(
		configPrefix+suffixBatchMaxMessages,
		defaultBatchingMaxMessages,
		"(experimental) Number of messages to batch before sending records to Pulsar. Higher value reduce request to pulsar but increase latency and the possibility of data loss in case of process restart.",
	)
	flagSet.String(
		configPrefix+suffixURL,
		defaultURL,
		"The comma-separated list of pulsar url. i.e. 'pulsar://127.0.0.1:6650'")
	flagSet.String(
		configPrefix+suffixTopic,
		defaultTopic,
		"The name of the pulsar topic")
	flagSet.String(
		configPrefix+suffixEncoding,
		defaultEncoding,
		fmt.Sprintf(`Encoding of spans ("%s" or "%s") sent to pulsar.`, EncodingJSON, EncodingProto),
	)
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	compressionMode := strings.ToLower(v.GetString(configPrefix + suffixCompression))
	compressionModeCodec, err := getCompressionMode(compressionMode)
	if err != nil {
		log.Fatal(err)
	}

	compressionLevel, err := getCompressionLevel(compressionMode, v.GetInt(configPrefix+suffixCompressionLevel))
	if err != nil {
		log.Fatal(err)
	}

	opt.Config = producer.Configuration{
		URL:                     v.GetString(configPrefix + suffixURL),
		Topic:                   v.GetString(configPrefix + suffixTopic),
		Token:                   v.GetString(configPrefix + suffixToken),
		Compression:             compressionModeCodec,
		CompressionLevel:        compressionLevel,
		BatchingMaxPublishDelay: v.GetDuration(configPrefix + suffixBatchLinger),
		BatchingMaxSize:         v.GetInt(configPrefix + suffixBatchSize),
		BatchingMaxMessages:     v.GetInt(configPrefix + suffixBatchMaxMessages),
	}
	opt.Encoding = v.GetString(configPrefix + suffixEncoding)
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.ReplaceAll(str, " ", "")
}

// getCompressionLevel to get compression level from compression type
func getCompressionLevel(mode string, compressionLevel int) (pulsar.CompressionLevel, error) {
	compressionModeData, ok := compressionModes[mode]
	if !ok {
		return 0, fmt.Errorf("cannot find compression mode for compressionMode %v", mode)
	}

	if compressionLevel == defaultCompressionLevel {
		return compressionModeData.defaultCompressionLevel, nil
	}

	if int(compressionModeData.minCompressionLevel) > compressionLevel || int(compressionModeData.maxCompressionLevel) < compressionLevel {
		return 0, fmt.Errorf("compression level %d for '%s' is not within valid range [%d, %d]", compressionLevel, mode, compressionModeData.minCompressionLevel, compressionModeData.maxCompressionLevel)
	}

	return pulsar.CompressionLevel(compressionLevel), nil
}

// getCompressionMode maps input modes to pulsar CompressionCodec
func getCompressionMode(mode string) (pulsar.CompressionType, error) {
	compressionMode, ok := compressionModes[mode]
	if !ok {
		return 0, fmt.Errorf("unknown compression mode: %v", mode)
	}

	return compressionMode.compressor, nil
}
