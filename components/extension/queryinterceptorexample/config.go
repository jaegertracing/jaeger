// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample

// Config configures the example query-interceptor extension. It decides per
// caller: the caller's role is read from a request header (propagated into the
// context as OTel client metadata), and privileged callers bypass the
// restrictions. A real extension would resolve the caller against a policy /
// authorization system rather than matching a static role list.
type Config struct {
	// IdentityHeader is the request header carrying the caller's role/identity.
	// It must be exposed to the context via jaeger_query's http.include_metadata.
	IdentityHeader string `mapstructure:"identity_header"`
	// PrivilegedRoles are IdentityHeader values that bypass all restrictions
	// (they see unredacted results and may filter on any attribute).
	PrivilegedRoles []string `mapstructure:"privileged_roles"`
	// DenyQueryAttributes lists attribute keys a non-privileged caller may not
	// filter on. If such a query references any of them, OnQuery rejects it —
	// demonstrating per-caller query-time admission (the pre-query hook).
	DenyQueryAttributes []string `mapstructure:"deny_query_attributes"`
	// RedactAttributes lists span attribute keys whose values OnResult replaces
	// with a placeholder for non-privileged callers — demonstrating per-caller
	// return-path masking (the post hook).
	RedactAttributes []string `mapstructure:"redact_attributes"`
}
