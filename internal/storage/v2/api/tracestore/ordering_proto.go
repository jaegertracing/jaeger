// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"

	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
)

// SpanOrderFromProto converts API v3 or storage v2 SpanSortOrder messages without validation or defaulting.
// The type parameter lets both generated protobuf types use the same decoder.
func SpanOrderFromProto[T interface {
	GetExpression() *expressionproto.Expression
	GetDirection() string
}](terms []T) ([]SpanSortOrder, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	order := make([]SpanSortOrder, len(terms))
	for i, term := range terms {
		expr, err := expressionproto.FromProto(term.GetExpression())
		if err != nil {
			return nil, fmt.Errorf("cannot decode order_by[%d]: %w", i, err)
		}
		order[i] = SpanSortOrder{Expression: expr, Direction: SortDirection(term.GetDirection())}
	}
	return order, nil
}
