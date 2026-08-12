// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"errors"
	"fmt"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// ErrFilterUnsupported is returned for a well-formed query filter that this
// deployment's storage cannot serve — a level it does not index, an operator it has
// not implemented, or a boolean structure a flat index cannot evaluate (RFC 0005 §7).
// The query is refused rather than approximated, so a caller never reads a narrower
// answer as the whole one.
var ErrFilterUnsupported = errors.New("this storage backend cannot serve this query filter")

// ErrFilterInvalid is returned for a query filter whose value does not fit the field it
// compares — the kind of mistake a structural check cannot catch, because the filter AST
// deliberately does not carry types (RFC 0005 §6.1).
var ErrFilterInvalid = errors.New("invalid query filter")

// IsBadRequest reports whether err means the caller must change the query, either
// because its shape is wrong or because this deployment's storage cannot serve it.
// Either way it is the caller's problem, so the API layers answer InvalidArgument /
// HTTP 400 rather than reporting a server fault.
func IsBadRequest(err error) bool {
	return errors.Is(err, ErrServiceNameRequired) ||
		errors.Is(err, ErrFilterUnsupported) ||
		errors.Is(err, ErrFilterInvalid) ||
		errors.Is(err, queryinterceptor.ErrFilterNotInterceptable)
}

// prepareFilter decides what a reader receives for a search that carries a structured
// filter. A reader that declares filter support gets the filter itself, once every level
// and operator it uses is one that reader listed. A reader that declares none — every
// backend until per-backend routing lands, and any remote plugin that predates the
// capability — gets the filter rewritten into the legacy predicate fields, which carry
// the equalities and inclusive duration bounds and nothing else (filter_legacy.go).
func prepareFilter(query TraceQueryParams, caps tracestore.SearchCapabilities) (TraceQueryParams, error) {
	if err := checkNoLegacyPredicates(query); err != nil {
		return query, err
	}
	if caps.Filter == nil {
		return rewriteFilterAsLegacyFields(query)
	}
	return query, checkFilterSupported(query.Filter, *caps.Filter)
}

// checkNoLegacyPredicates rejects a query that carries both a filter and one of the
// predicate fields the filter replaces. The two express the same things — a service, an
// operation name, a duration bound, a tag — so honoring both would leave the caller
// guessing which one applied.
func checkNoLegacyPredicates(query TraceQueryParams) error {
	var set []string
	if query.ServiceName != "" {
		set = append(set, "service_name")
	}
	if query.OperationName != "" {
		set = append(set, "operation_name")
	}
	if query.DurationMin != 0 {
		set = append(set, "duration_min")
	}
	if query.DurationMax != 0 {
		set = append(set, "duration_max")
	}
	if query.Attributes.Len() > 0 {
		set = append(set, "attributes")
	}
	if len(set) == 0 {
		return nil
	}
	return fmt.Errorf("%w: it cannot be combined with %v; express those predicates in the filter instead",
		ErrFilterInvalid, set)
}

// checkFilterSupported walks the filter and refuses the first predicate the reader did
// not declare it can evaluate.
func checkFilterSupported(filter *tracestore.Call, caps tracestore.FilterCapabilities) error {
	if !caps.SupportsOperator(filter.Op) {
		return errUnsupportedOperator(filter.Op)
	}
	for _, arg := range filter.Args {
		switch term := arg.(type) {
		case *tracestore.Call:
			if !caps.Boolean && isBoolean(filter.Op) && isBoolean(term.Op) {
				return errNestedBoolean()
			}
			if err := checkFilterSupported(term, caps); err != nil {
				return err
			}
		case *tracestore.Reference:
			if !caps.SupportsLevel(term.Level) {
				return fmt.Errorf("%w: it does not index the %q level", ErrFilterUnsupported, term.Level)
			}
		default:
			// A constant carries nothing a reader has to support.
		}
	}
	return nil
}

func isBoolean(op tracestore.Operator) bool {
	return op == tracestore.OpAnd || op == tracestore.OpOr || op == tracestore.OpNot
}

func errUnsupportedOperator(op tracestore.Operator) error {
	return fmt.Errorf("%w: it does not support the operator %q", ErrFilterUnsupported, op)
}

func errNestedBoolean() error {
	return fmt.Errorf("%w: it evaluates a flat conjunction only, not nested boolean groups", ErrFilterUnsupported)
}
