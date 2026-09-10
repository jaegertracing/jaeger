// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// A filter arrives over two wires — an api_v3 request and the remote-storage protocol — and both
// owe a Reader the same query: one filtering model rather than two, and nothing the Reader did not
// declare it can evaluate (RFC 0005 §7). The checks live here, beside the query type and the
// capability declaration, so each wire runs the same ones rather than its own.

// EnsureFilterStandsAlone rejects a query that carries both a filter and one of the predicate
// fields the filter replaces. The two express the same things — a service, an operation name, a
// duration bound, a tag — so honoring both would leave the caller guessing which one applied.
func (q TraceQueryParams) EnsureFilterStandsAlone() error {
	if q.Filter == nil {
		return nil
	}
	var set []string
	if q.ServiceName != "" {
		set = append(set, "service_name")
	}
	if q.OperationName != "" {
		set = append(set, "operation_name")
	}
	if q.DurationMin != 0 {
		set = append(set, "duration_min")
	}
	if q.DurationMax != 0 {
		set = append(set, "duration_max")
	}
	if q.Attributes != (pcommon.Map{}) && q.Attributes.Len() > 0 {
		set = append(set, "attributes")
	}
	if len(set) == 0 {
		return nil
	}
	return fmt.Errorf("%w: it cannot be combined with %v; express those predicates in the filter instead",
		ErrFilterInvalid, set)
}

// ForCapabilities gives the Reader whichever of the two filtering models it declared it can
// evaluate. A Reader that declares filter support gets the filter itself, once every level and
// operator it uses is one that Reader listed. A Reader that declares none gets the filter rewritten
// into the legacy predicate fields, which carry the equalities and inclusive duration bounds and
// nothing else (ToLegacyShape), or a refusal where they cannot carry it.
//
// It answers only that question. Whether the request is one this deployment accepts at all is the
// caller's to settle first.
func (q TraceQueryParams) ForCapabilities(caps SearchCapabilities) (TraceQueryParams, error) {
	if q.Filter == nil {
		return q, nil
	}
	if caps.Filter.IsEmpty() {
		return q.ToLegacyShape()
	}
	return q, caps.Filter.EnsureSupported(q.Filter)
}

// EnsureSupported walks the filter and refuses the first predicate the Reader did not declare it
// can evaluate.
func (c FilterCapabilities) EnsureSupported(filter *expression.Call) error {
	if filter == nil {
		return nil
	}
	if !c.SupportsOperator(filter.Op) {
		return fmt.Errorf("%w: it does not support the operator %q", ErrFilterUnsupported, filter.Op)
	}
	for _, arg := range filter.Args {
		var level expression.Level
		switch term := arg.(type) {
		case *expression.Call:
			if err := c.EnsureSupported(term); err != nil {
				return err
			}
			continue
		case *expression.AttributeRef:
			level = term.Level
		case *expression.FieldRef:
			level = term.Level
		case *expression.NestedRef:
			level = term.Level
		default:
			// A constant carries nothing a Reader has to support.
			continue
		}
		if !c.SupportsLevel(level) {
			return fmt.Errorf("%w: it does not index the %q level", ErrFilterUnsupported, level)
		}
	}
	return nil
}
