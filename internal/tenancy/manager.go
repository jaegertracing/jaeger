// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

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

func (tenantDontCare) Valid(string /* candidate */) bool {
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
