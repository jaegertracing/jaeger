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

package nats

import (
	"flag"
	"fmt"
	"strings"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/nats/auth"
	"github.com/jaegertracing/jaeger/pkg/nats/producer"
)

const (
	// EncodingJSON is used for spans encoded as Protobuf-based JSON.
	EncodingJSON = "json"
	// EncodingProto is used for spans encoded as Protobuf.
	EncodingProto = "protobuf"
	// EncodingZipkinThrift is used for spans encoded as Zipkin Thrift.
	EncodingZipkinThrift = "zipkin-thrift"

	configPrefix           = "nats.producer"
	suffixServers          = ".servers"
	suffixTopic            = ".subject"
	suffixEncoding         = ".encoding"
	suffixRequiredAcks     = ".required-acks"

	defaultServer           = "127.0.0.1:9092"
	defaultSubject          = "jaeger-spans"
	defaultEncoding         = EncodingProto
	defaultRequiredAcks     = "local"
)

var (
	// AllEncodings is a list of all supported encodings.
	AllEncodings = []string{EncodingJSON, EncodingProto, EncodingZipkinThrift}

)

// Options stores the configuration options for NATS
type Options struct {
	config   producer.Configuration
	subject  string
	encoding string
}

// AddFlags adds flags for Options
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		configPrefix+suffixServers,
		defaultServer,
		"The comma-separated list of NATS servers. i.e. '127.0.0.1:9092,0.0.0:1234'")
	flagSet.String(
		configPrefix+suffixTopic,
		defaultSubject,
		"The name of the NATS subject")
	flagSet.String(
		configPrefix+suffixEncoding,
		defaultEncoding,
		fmt.Sprintf(`Encoding of spans ("%s" or "%s") sent to NATS.`, EncodingJSON, EncodingProto),
	)
	flagSet.String(
		configPrefix+suffixRequiredAcks,
		defaultRequiredAcks,
		"(experimental) Required NATS broker acknowledgement. i.e. noack, local, all",
	)
	auth.AddFlags(configPrefix, flagSet)
}

// InitFromViper initializes Options with properties from viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	authenticationOptions := auth.AuthenticationConfig{}
	authenticationOptions.InitFromViper(configPrefix, v)

	opt.config = producer.Configuration{
		Servers:              stripWhiteSpace(v.GetString(configPrefix+suffixServers)),
		AuthenticationConfig: authenticationOptions,
	}
	opt.subject = v.GetString(configPrefix + suffixTopic)
	opt.encoding = v.GetString(configPrefix + suffixEncoding)
}

// stripWhiteSpace removes all whitespace characters from a string
func stripWhiteSpace(str string) string {
	return strings.Replace(str, " ", "", -1)
}

