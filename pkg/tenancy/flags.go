// Copyright (c) 2022 The Jaeger Authors.
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

package tenancy

import (
	"flag"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

const (
	flagPrefix         = "multi-tenancy"
	flagTenancyEnabled = flagPrefix + ".enabled"
	flagTenancyHeader  = flagPrefix + ".header"
	flagValidTenants   = flagPrefix + ".tenants"
)

// AddFlags adds flags for tenancy to the FlagSet.
func AddFlags(flags *flag.FlagSet) {
	flags.Bool(flagTenancyEnabled, false, "Enable tenancy header when receiving or querying")
	flags.String(flagTenancyHeader, "x-tenant", "HTTP header carrying tenant")
	flags.String(flagValidTenants, "",
		fmt.Sprintf("comma-separated list of allowed values for --%s header.  (If not supplied, tenants are not restricted)",
			flagTenancyHeader))
}

// InitFromViper creates tenancy.Options populated with values retrieved from Viper.
func InitFromViper(v *viper.Viper) Options {
	var p Options
	p.Enabled = v.GetBool(flagTenancyEnabled)
	p.Header = v.GetString(flagTenancyHeader)
	tenants := v.GetString(flagValidTenants)
	if len(tenants) != 0 {
		p.Tenants = strings.Split(tenants, ",")
	} else {
		p.Tenants = []string{}
	}

	return p
}
