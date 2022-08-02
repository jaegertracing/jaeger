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

// Options describes the configuration properties for multitenancy
type Options struct {
	Enabled bool
	Header  string
	Tenants []string
}

// Manager can check tenant usage for multi-tenant Jaeger configurations
type Manager struct {
	Enabled bool
	Header  string
	guard   guard
}

// Guard verifies a valid tenant when tenancy is enabled
type guard interface {
	Valid(candidate string) bool
}

// NewManager creates a tenancy.Manager for given tenancy.Options.
func NewManager(options *Options) *Manager {
	// Default header value (although set by CLI flags, this helps tests and API users)
	header := options.Header
	if header == "" && options.Enabled {
		header = "x-tenant"
	}
	return &Manager{
		Enabled: options.Enabled,
		Header:  header,
		guard:   tenancyGuardFactory(options),
	}
}

func (tc *Manager) Valid(tenant string) bool {
	return tc.guard.Valid(tenant)
}

type tenantDontCare bool

func (tenantDontCare) Valid(candidate string) bool {
	return true
}

type tenantList struct {
	tenants map[string]bool
}

func (tl *tenantList) Valid(candidate string) bool {
	_, ok := tl.tenants[candidate]
	return ok
}

func newTenantList(tenants []string) *tenantList {
	tenantMap := make(map[string]bool)
	for _, tenant := range tenants {
		tenantMap[tenant] = true
	}

	return &tenantList{
		tenants: tenantMap,
	}
}

func tenancyGuardFactory(options *Options) guard {
	// Three cases
	// - no tenancy
	// - tenancy, but no guarding by tenant
	// - tenancy, with guarding by a list

	if !options.Enabled || len(options.Tenants) == 0 {
		return tenantDontCare(true)
	}

	return newTenantList(options.Tenants)
}
