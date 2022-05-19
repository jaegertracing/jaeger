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

// TenancyConfig holds the settings for multi-tenant Jaeger
type TenancyConfig struct {
	Enabled bool
	Header  string
	Valid   Guard
}

// Guard verifies a valid tenant when tenancy is enabled
type Guard interface {
	Valid(candidate string) bool
}

// Options describes the configuration properties for multitenancy
type Options struct {
	Enabled bool     `mapstructure:"enabled"`
	Header  string   `mapstructure:"header"`
	Tenants []string `mapstructure:"tenants"`
}

// NewTenancyConfig creates a tenancy configuration for tenancy Options
func NewTenancyConfig(options *Options) *TenancyConfig {
	return &TenancyConfig{
		Enabled: options.Enabled,
		Header:  options.Header,
		Valid:   TenancyGuardFactory(options),
	}
}

// @@@ ecs TODO a function that validates Options; and call it

type tenantUnaware bool

func (tenantUnaware) Valid(candidate string) bool {
	return true
}

type tenantList struct {
	tenants map[string]bool
}

func (tl tenantList) Valid(candidate string) bool {
	_, ok := tl.tenants[candidate]
	return ok
}

func newTenantList(tenants []string) tenantList {
	tenantMap := make(map[string]bool)
	for _, tenant := range tenants {
		tenantMap[tenant] = true
	}

	return tenantList{
		tenants: tenantMap,
	}
}

// @@@ ecs
func TenancyGuardFactory(options *Options) Guard {
	if !options.Enabled {
		return tenantUnaware(true)
	}

	return newTenantList(options.Tenants)
}
