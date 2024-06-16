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

package memory

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	limit                      = "memory.max-traces"
	samplingAggregationBuckets = "memory.sampling.aggregation-buckets"
)

// Options stores the configuration entries for this storage
type Options struct {
	Config Configuration `mapstructure:",squash"`
}

// AddFlags from this storage to the CLI
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(limit, 0, "The maximum amount of traces to store in memory. The default number of traces is unbounded.")
	flagSet.Int(samplingAggregationBuckets, 20, "SamplingAggregationBuckets is used with adaptive sampling to control how many buckets of trace throughput is stored in memory. Should not be fewer than the number of buckets used in adaptive sampling.")
}

// InitFromViper initializes the options struct with values from Viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.Config.MaxTraces = v.GetInt(limit)
	opt.Config.SamplingAggregationBuckets = v.GetInt(samplingAggregationBuckets)
}
