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

package app

import (
	"flag"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/kafka/auth"
)

const (
	// ConfigPrefix is a prefix for the ingester flags
	ConfigPrefix           = "benchmark"
	SuffixNumberOfTraces   = ".traces-number"
	SuffixNumberOfProcess  = ".process.max-number"
	SuffixMinNumberOfSpans = ".spans.min-number"
	SuffixMaxNumberOfSpans = ".spans.max-number"
	SuffixMinTags          = ".tags.min-number"
	SuffixMaxTags          = ".tags.max-number"

	DefaultNumberOfTraces   = 5000
	DefaultNumberOfProcess  = 50
	DefaultMinNumberOfSpans = 15
	DefaultMaxNumberOfSpans = 50
	DefaultMinTags          = 10
	DefaultMaxTags          = 120
)

type Options struct {
	TracesNumber  int
	ProcessNumber int
	SpanMinNumber int
	SpanMaxNumber int
	TagsMinNumber int
	TagsMaxNumber int
}

// AddFlags adds flags for Builder
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(
		ConfigPrefix+SuffixNumberOfTraces,
		DefaultNumberOfTraces,
		"Number of traces to be generated for the test")
	flagSet.Int(
		ConfigPrefix+SuffixNumberOfProcess,
		DefaultNumberOfProcess,
		"Number of traces to be generated for the test")
	flagSet.Int(
		ConfigPrefix+SuffixMinNumberOfSpans,
		DefaultMinNumberOfSpans,
		"Minimum number of spans per trace")
	flagSet.Int(
		ConfigPrefix+SuffixMaxNumberOfSpans,
		DefaultMaxNumberOfSpans,
		"Maximum number of spans per trace")
	flagSet.Int(
		ConfigPrefix+SuffixMinTags,
		DefaultMinTags,
		"Min number of tags per span or service")
	flagSet.Int(
		ConfigPrefix+SuffixMaxTags,
		DefaultMaxTags,
		"Max number of tags per span or service")
	// Authentication flags
	auth.AddFlags(ConfigPrefix, flagSet)
}

// InitFromViper initializes Builder with properties from viper
func (o *Options) InitFromViper(v *viper.Viper) {
	o.TracesNumber = v.GetInt(ConfigPrefix + SuffixNumberOfTraces)
	o.ProcessNumber = v.GetInt(ConfigPrefix + SuffixNumberOfProcess)
	o.SpanMinNumber = v.GetInt(ConfigPrefix + SuffixMinNumberOfSpans)
	o.SpanMaxNumber = v.GetInt(ConfigPrefix + SuffixMaxNumberOfSpans)
	o.TagsMinNumber = v.GetInt(ConfigPrefix + SuffixMinTags)
	o.TagsMaxNumber = v.GetInt(ConfigPrefix + SuffixMaxTags)
}
