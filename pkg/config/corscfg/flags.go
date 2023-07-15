// Copyright (c) 2023 The Jaeger Authors.
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

package corscfg

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	corsPrefix         = ".cors"
	corsAllowedHeaders = corsPrefix + ".allowed-headers"
	corsAllowedOrigins = corsPrefix + ".allowed-origins"
)

type Flags struct {
	Prefix string
}

func (c Flags) AddFlags(flags *flag.FlagSet) {
	flags.String(c.Prefix+corsAllowedHeaders, "content-type", "Allowed headers for the HTTP port , default content-type")
	flags.String(c.Prefix+corsAllowedOrigins, "*", "Allowed origins for the HTTP port , default accepts all")
}

func (c Flags) InitFromViper(v *viper.Viper) Options {
	var p Options
	p.AllowedHeaders = v.GetStringSlice(c.Prefix + corsAllowedHeaders)
	p.AllowedOrigins = v.GetStringSlice(c.Prefix + corsAllowedOrigins)
	return p
}
