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
	tenancyEnabled = "multi_tenancy.enabled"
	tenancyHeader  = "multi_tenancy.header"
	validTenants   = "multi_tenancy.tenants"
)

// TenancyFlagsConfig describes which CLI flags for TLS client should be generated.
type TenancyFlagsConfig struct {
}

// AddFlags adds flags for tenancy to the FlagSet.
func (c TenancyFlagsConfig) AddFlags(flags *flag.FlagSet) {
	flags.Bool(tenancyEnabled, false, "Enable tenancy header when receiving or querying")
	flags.String(tenancyHeader, "x-tenant", "HTTP header carrying tenant")
	flags.String(validTenants, "", "Acceptable tenants")
}

// InitFromViper creates tls.Config populated with values retrieved from Viper.
func (c TenancyFlagsConfig) InitFromViper(v *viper.Viper) (Options, error) {
	var p Options
	p.Enabled = v.GetBool(tenancyEnabled)
	p.Header = v.GetString(tenancyHeader)
	p.Tenants = strings.Split(v.GetString(validTenants), ",")

	if p.Enabled && len(p.Tenants) == 0 {
		return p, fmt.Errorf("%s must have at least one tenant when tenancy is enabled", validTenants)
	}

	return p, nil
}
