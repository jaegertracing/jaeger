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
	"strings"

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
	flags.String(c.Prefix+corsAllowedHeaders, "", "Comma-separated CORS allowed headers. See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Allow-Headers")
	flags.String(c.Prefix+corsAllowedOrigins, "", "Comma-separated CORS allowed origins. See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Allow-Origin")
}

func (c Flags) InitFromViper(v *viper.Viper) Options {
	var p Options

	allowedHeaders := v.GetString(c.Prefix + corsAllowedHeaders)
	allowedOrigins := v.GetString(c.Prefix + corsAllowedOrigins)

	p.AllowedOrigins = strings.Split(strings.ReplaceAll(allowedOrigins, " ", ""), ",")
	p.AllowedHeaders = strings.Split(strings.ReplaceAll(allowedHeaders, " ", ""), ",")

	return p
}
