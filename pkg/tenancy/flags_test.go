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
