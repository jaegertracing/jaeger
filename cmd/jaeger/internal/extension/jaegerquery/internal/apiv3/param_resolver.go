// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"net/http"
	"net/url"
)

// paramResolver resolves HTTP query parameters with canonical camelCase names and
// deprecated snake_case aliases. One instance is created per request.
type paramResolver struct {
	values     url.Values
	deprecated []string // nil until first deprecated param is matched (lazy alloc)
}

// newParamResolver constructs a resolver from the request query string.
func newParamResolver(r *http.Request) *paramResolver {
	return &paramResolver{values: r.URL.Query()}
}

// Resolve returns the resolved value, the exact parameter name that was present
// (canonical or deprecated), and whether a non-empty value was found.
//
// Precedence: when both canonical and deprecated forms are present, the canonical
// value is used and deprecated aliases are not recorded (no deprecation signal).
//
// Empty values (?param=) are treated as absent, matching url.Values.Get semantics.
// When multiple values are present (?startTime=a&startTime=b), the first value wins
// per url.Values.Get.
func (p *paramResolver) Resolve(canonical string, deprecated ...string) (value, actualName string, found bool) {
	if v := p.values.Get(canonical); v != "" {
		return v, canonical, true
	}
	for _, d := range deprecated {
		if v := p.values.Get(d); v != "" {
			p.recordDeprecated(d)
			return v, d, true
		}
	}
	return "", "", false
}

func (p *paramResolver) recordDeprecated(name string) {
	for _, existing := range p.deprecated {
		if existing == name {
			return
		}
	}
	if p.deprecated == nil {
		p.deprecated = make([]string, 0, 4)
	}
	p.deprecated = append(p.deprecated, name)
}

// DeprecatedParamsUsed returns deprecated parameter names matched during this request.
// Returns nil when no deprecated parameters were used.
func (p *paramResolver) DeprecatedParamsUsed() []string {
	return p.deprecated
}
