// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	expressionproto "github.com/jaegertracing/jaeger/internal/proto/expression/v1"
)

// SpanOrderFromProto decodes either API's sort terms and validates the shared ordering contract.
func SpanOrderFromProto[T interface {
	GetExpression() *expressionproto.Expression
	GetDirection() string
}](terms []T) ([]SpanSortOrder, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	order := make([]SpanSortOrder, len(terms))
	for i, term := range terms {
		field := term.GetExpression().GetField()
		if field == nil {
			return nil, fmt.Errorf("%w: order_by[%d] must reference a built-in span field", ErrSpanOrderInvalid, i)
		}
		order[i] = SpanSortOrder{Expression: &expression.FieldRef{Level: expression.Level(field.GetLevel()), Name: field.GetName()}, Direction: SortDirection(term.GetDirection())}
	}
	return NormalizeSpanOrder(order)
}

// SpanOrderExpression encodes a normalized sort term for either storage or API v3.
func SpanOrderExpression(term SpanSortOrder) *expressionproto.Expression {
	ref := term.Expression.(*expression.FieldRef)
	return &expressionproto.Expression{Term: &expressionproto.Expression_Field{Field: &expressionproto.FieldReference{Level: string(ref.Level), Name: ref.Name}}}
}
