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

package kafkaexporter

import (
	"flag"
	"fmt"

	"github.com/jaegertracing/jaeger/plugin/storage/kafka"
)

const (
	// encodingOTLPProto is used for spans encoded as OTLP Protobuf.
	encodingOTLPProto = "otlp-proto"
)

// AddFlags adds Ingester flags.
func AddFlags(flags *flag.FlagSet) {
	opts := &kafka.Options{}
	opts.AddOTELFlags(flags)
	// Modify kafka.producer.encoding flag
	flags.Lookup("kafka.producer.encoding").Usage = fmt.Sprintf(
		`Encoding of spans ("%s", "%s" or "%s") sent to kafka.`,
		kafka.EncodingJSON,
		kafka.EncodingProto,
		encodingOTLPProto,
	)
}
