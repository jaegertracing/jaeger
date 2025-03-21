// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tenancy

import (
	"flag"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTenancyFlags(t *testing.T) {
	tests := []struct {
		name     string
		cmd      []string
		expected Options
	}{
		{
			name: "one tenant",
			cmd: []string{
				"--multi-tenancy.enabled=true",
				"--multi-tenancy.tenants=acme",
			},
			expected: Options{
				Enabled: true,
				Header:  "x-tenant",
				Tenants: []string{"acme"},
			},
		},
		{
			name: "two tenants",
			cmd: []string{
				"--multi-tenancy.enabled=true",
				"--multi-tenancy.tenants=acme,country-store",
			},
			expected: Options{
				Enabled: true,
				Header:  "x-tenant",
				Tenants: []string{"acme", "country-store"},
			},
		},
		{
			name: "custom header",
			cmd: []string{
				"--multi-tenancy.enabled=true",
				"--multi-tenancy.header=jaeger-tenant",
				"--multi-tenancy.tenants=acme",
			},
			expected: Options{
				Enabled: true,
				Header:  "jaeger-tenant",
				Tenants: []string{"acme"},
			},
		},
		{
			// Not supplying a list of tenants will mean
			// "tenant header required, but any value will pass"
			name: "no_tenants",
			cmd: []string{
				"--multi-tenancy.enabled=true",
			},
			expected: Options{
				Enabled: true,
				Header:  "x-tenant",
				Tenants: []string{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			v := viper.New()
			command := cobra.Command{}
			flagSet := &flag.FlagSet{}
			AddFlags(flagSet)
			command.PersistentFlags().AddGoFlagSet(flagSet)
			v.BindPFlags(command.PersistentFlags())

			err := command.ParseFlags(test.cmd)
			require.NoError(t, err)
			tenancyCfg := InitFromViper(v)
			assert.Equal(t, test.expected, tenancyCfg)
		})
	}
}
