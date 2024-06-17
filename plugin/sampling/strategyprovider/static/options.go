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

package static

import (
	"flag"
	"time"

	"github.com/spf13/viper"
)

const (
	// samplingStrategiesFile contains the name of CLI option for config file.
	samplingStrategiesFile           = "sampling.strategies-file"
	samplingStrategiesReloadInterval = "sampling.strategies-reload-interval"
	samplingStrategiesBugfix5270     = "sampling.strategies.bugfix-5270"
)

// Options holds configuration for the static sampling strategy store.
type Options struct {
	// StrategiesFile is the path for the sampling strategies file in JSON format
	StrategiesFile string
	// ReloadInterval is the time interval to check and reload sampling strategies file
	ReloadInterval time.Duration
	// Flag for enabling possibly breaking change which includes default operations level
	// strategies when calculating Ratelimiting type service level strategy
	// more information https://github.com/jaegertracing/jaeger/issues/5270
	IncludeDefaultOpStrategies bool
}

// AddFlags adds flags for Options
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Duration(samplingStrategiesReloadInterval, 0, "Reload interval to check and reload sampling strategies file. Zero value means no reloading")
	flagSet.String(samplingStrategiesFile, "", "The path for the sampling strategies file in JSON format. See sampling documentation to see format of the file")
	flagSet.Bool(samplingStrategiesBugfix5270, false, "Include default operation level strategies for Ratesampling type service level strategy. Cf. https://github.com/jaegertracing/jaeger/issues/5270")
}

// InitFromViper initializes Options with properties from viper
func (opts *Options) InitFromViper(v *viper.Viper) *Options {
	opts.StrategiesFile = v.GetString(samplingStrategiesFile)
	opts.ReloadInterval = v.GetDuration(samplingStrategiesReloadInterval)
	opts.IncludeDefaultOpStrategies = v.GetBool(samplingStrategiesBugfix5270)
	return opts
}
