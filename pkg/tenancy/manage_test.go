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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTenancyValidity(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		tenant  string
		valid   bool
	}{
		{
			name: "valid single tenant",
			options: Options{
				Enabled: true,
				Header:  "x-tenant",
				Tenants: []string{"acme"},
			},
			tenant: "acme",
			valid:  true,
		},
		{
			name: "valid tenant in multi-tenant setup",
			options: Options{
				Enabled: true,
				Header:  "x-tenant",
				Tenants: []string{"acme", "country-store"},
			},
			tenant: "acme",
			valid:  true,
		},
		{
			name: "invalid tenant",
			options: Options{
				Enabled: true,
				Header:  "x-tenant",
				Tenants: []string{"acme", "country-store"},
			},
			tenant: "auto-repair",
			valid:  false,
		},
		{
			// Not supplying a list of tenants will mean
			// "tenant header required, but any value will pass"
			name: "any tenant",
			options: Options{
				Enabled: true,
				Header:  "x-tenant",
				Tenants: []string{},
			},
			tenant: "convenience-store",
			valid:  true,
		},
		{
			name: "ignore tenant",
			options: Options{
				Enabled: false,
				Header:  "",
				Tenants: []string{"acme"},
			},
			tenant: "country-store",
			// If tenancy not enabled, any tenant is valid
			valid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tc := NewManager(&test.options)
			assert.Equal(t, test.valid, tc.Valid(test.tenant))
		})
	}
}
