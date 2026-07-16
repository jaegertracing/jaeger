// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample

// Config configures the example query-interceptor extension. The two fields
// exercise the two hooks of queryinterceptor.Interceptor; the real thing would
// consult a policy system (e.g. Gandalf) instead of static lists.
type Config struct {
	// DenyQueryAttributes lists attribute keys that a query is not allowed to
	// filter on. If a trace query references any of them, OnQuery rejects it —
	// demonstrating query-time admission (the pre-query hook).
	DenyQueryAttributes []string `mapstructure:"deny_query_attributes"`
	// RedactAttributes lists span attribute keys whose values OnResult replaces
	// with a placeholder — demonstrating return-path masking (the post hook).
	RedactAttributes []string `mapstructure:"redact_attributes"`
}
