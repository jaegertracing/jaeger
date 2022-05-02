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
	"strings"

	"github.com/spf13/viper"

	"github.com/jaegertracing/jaeger/pkg/memory/config"
)

const limit = "memory.max-traces"
const limitTenants = "memory.max-tenants"
const validTenants = "memory.valid-tenants"

// Options stores the configuration entries for this storage
type Options struct {
	Configuration config.Configuration `mapstructure:",squash"`
}

// AddFlags from this storage to the CLI
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.Int(limit, 0, "The maximum amount of traces to store in memory. The default number of traces is unbounded.")
	flagSet.Int(limitTenants, 10, "limit on the number of tenants that may send and query (use 0 for no limit)")
	flagSet.String(validTenants, "", "If defined; only tenants in this list may send and query")
}

// InitFromViper initializes the options struct with values from Viper
func (opt *Options) InitFromViper(v *viper.Viper) {
	opt.Configuration.MaxTraces = v.GetInt(limit)
	opt.Configuration.MaxTenants = v.GetInt(limitTenants)
	opt.Configuration.ValidTenants = strings.Split(v.GetString(validTenants), ",")
}
