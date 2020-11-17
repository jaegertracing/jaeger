// Copyright (c) 2020 The Jaeger Authors.
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

package kafkareceiver

import (
	"flag"
	"fmt"
	"strings"

	ingesterApp "github.com/jaegertracing/jaeger/cmd/ingester/app"
	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

const (
	// encodingZipkinProto is used for spans encoded as Zipkin Protobuf.
	encodingZipkinProto = "zipkin-proto"
	// encodingZipkinJSON is used for spans encoded as Zipkin JSON.
	encodingZipkinJSON = "zipkin-json"
	// encodingOTLPProto is used for spans encoded as OTLP Protobuf.
	encodingOTLPProto = "otlp-proto"
)

// AddFlags adds Ingester flags.
func AddFlags(flags *flag.FlagSet) {
	ingesterApp.AddOTELFlags(flags)
	// Modify kafka.consumer.encoding flag
	flags.Lookup(ingesterApp.KafkaConsumerConfigPrefix + ingesterApp.SuffixEncoding).Usage = fmt.Sprintf(`The encoding of spans ("%s") consumed from kafka`, strings.Join(
		append(kafka.AllEncodings,
			encodingZipkinJSON,
			encodingZipkinProto,
			encodingOTLPProto,
		), "\", \""))
}
