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

	"github.com/jaegertracing/jaeger/cmd/ingester/app/builder"
)

// AddFlags adds flags for Builder
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(
		builder.ConfigPrefix+builder.SuffixBrokers,
		builder.DefaultBroker,
		"The comma-separated list of kafka brokers. i.e. '127.0.0.1:9092,0.0.0:1234'")
	flagSet.String(
		builder.ConfigPrefix+builder.SuffixTopic,
		builder.DefaultTopic,
		"The name of the kafka topic to consume from")
	flagSet.String(
		builder.ConfigPrefix+builder.SuffixGroupID,
		builder.DefaultGroupID,
		"The Consumer Group that ingester will be consuming on behalf of")
	flagSet.String(
		builder.ConfigPrefix+builder.SuffixParallelism,
		strconv.Itoa(builder.DefaultParallelism),
		"The number of messages to process in parallel")
	flagSet.String(
		builder.ConfigPrefix+builder.SuffixEncoding,
		builder.DefaultEncoding,
		fmt.Sprintf(`The encoding of spans ("%s" or "%s") consumed from kafka`, builder.EncodingProto, builder.EncodingJSON))
}
