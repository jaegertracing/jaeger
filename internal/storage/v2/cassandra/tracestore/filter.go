// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// FilterCapabilities declares the part of the RFC 0005 filter model Cassandra's flat,
// service-keyed index can serve: an equality conjunction over the three levels it indexes
// (span, resource and event attributes), merged into one undifferentiated tag index with no
// level of their own (RFC 0005 §1.6). A predicate naming the scope or link level, or using any
// operator other than `and`/`eq`, has no index to answer it and is refused rather than
// approximated.
func FilterCapabilities() tracestore.FilterCapabilities {
	return tracestore.FilterCapabilities{
		Levels: []expression.Level{
			expression.LevelSpan,
			expression.LevelResource,
			expression.LevelEvent,
		},
		Operators: []expression.Operator{
			expression.OpAnd,
			expression.OpEq,
		},
	}
}

// lowerFilter converts an incoming structured filter into the legacy predicate fields the v1
// reader below understands. Declaring FilterCapabilities is what stops the query service from
// doing this conversion on the reader's behalf (RFC 0005 §7): it sends the filter itself to any
// backend that declares support for it, so a reader whose v1 query methods have no code that
// reads TraceQueryParams.Filter at all has to convert incoming queries itself instead, the same
// way the query service otherwise would. ToLegacyShape refuses what it cannot carry (`or`/`not`,
// a level never indexed here, an operator other than `eq`) rather than silently dropping it, so
// a query carrying one of those is never answered with the wrong rows.
func lowerFilter(query tracestore.TraceQueryParams) (tracestore.TraceQueryParams, error) {
	if query.Filter == nil {
		return query, nil
	}
	return query.ToLegacyShape()
}
