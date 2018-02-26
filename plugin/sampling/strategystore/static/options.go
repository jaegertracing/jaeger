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

	"github.com/spf13/viper"
)

const (
	samplingStrategiesFile = "sampling.strategies-file"
)

// Options holds configuration for the static sampling strategy store.
type Options struct {
	// StrategiesFile is the path for the sampling strategies file in JSON format
	StrategiesFile string
}

// AddFlags adds flags for Options
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(samplingStrategiesFile, "", "The path for the sampling strategies file in JSON format")
}

// InitFromViper initializes Options with properties from viper
func (opts *Options) InitFromViper(v *viper.Viper) *Options {
	opts.StrategiesFile = v.GetString(samplingStrategiesFile)
	return opts
}
