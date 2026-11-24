// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// FinalizeFilter prepares a decoded filter for everything downstream of it: it checks the structure,
// reads every constant against the field it is compared to, and puts the reference first in each
// comparison. What it returns is a new tree; the one it was given is untouched.
//
// Running it again on its own result changes nothing, which is what lets each boundary finalize a
// filter it did not build — the query service after an interceptor has edited one, and the
// remote-storage server on whatever a client sent it (RFC 0005 §7).
func FinalizeFilter(filter *expression.Call) (*expression.Call, error) {
	if err := ValidateFilter(filter); err != nil {
		return nil, err
	}
	return ResolveFilterConstants(filter)
}
