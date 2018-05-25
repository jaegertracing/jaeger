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

	"github.com/jaegertracing/jaeger/pkg/memory/config"
)

const limit = "memory.limit"

// Options stores the configuration entries for this storage
type Options struct {
	Configuration config.Configuration
}

// AddFlags from this storage to the CLI
func (opt *Options) AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(limit, opt.Configuration.Limit, "The maximum amount of traces to store in memory")
}

// InitFromViper initializes the options struct with values from Viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.Configuration.Limit = v.GetInt(limit)
}
