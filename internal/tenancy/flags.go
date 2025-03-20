// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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
