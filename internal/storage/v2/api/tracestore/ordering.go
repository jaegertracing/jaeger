// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"fmt"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

type SortDirection string

const (
	SortAscending  SortDirection = "asc"
	SortDescending SortDirection = "desc"
)

// SpanSortOrder selects a span field and its direction, in query precedence order.
type SpanSortOrder struct {
	Expression expression.Expression
	Direction  SortDirection
}

// NormalizeSpanOrder validates explicit terms without adding implicit tie-breakers.
// An omitted order stays empty so readers can distinguish it from an explicit default.
func NormalizeSpanOrder(order []SpanSortOrder) ([]SpanSortOrder, error) {
	if len(order) == 0 {
		return nil, nil
	}
	out := make([]SpanSortOrder, len(order))
	seen := make(map[string]bool, len(order))
	for i, term := range order {
		ref, ok := term.Expression.(*expression.FieldRef)
		if !ok || ref == nil || ref.Level != expression.LevelSpan {
			return nil, fmt.Errorf("%w: order_by[%d] must reference a built-in span field", ErrSpanOrderInvalid, i)
		}
		switch ref.Name {
		case "startTime", "duration", "traceID", "spanID":
		default:
			return nil, fmt.Errorf("%w: order_by[%d] field %q is unsupported", ErrSpanOrderInvalid, i, ref.Name)
		}
		if seen[ref.Name] {
			return nil, fmt.Errorf("%w: order_by[%d] repeats field %q", ErrSpanOrderInvalid, i, ref.Name)
		}
		seen[ref.Name] = true
		direction := term.Direction
		if direction == "" {
			direction = SortAscending
		}
		if direction != SortAscending && direction != SortDescending {
			return nil, fmt.Errorf("%w: order_by[%d] direction %q is unsupported", ErrSpanOrderInvalid, i, direction)
		}
		out[i] = SpanSortOrder{Expression: &expression.FieldRef{Level: ref.Level, Name: ref.Name}, Direction: direction}
	}
	return out, nil
}

// EffectiveSpanOrder is the order a storage backend executes: the explicit terms, validated by
// NormalizeSpanOrder, followed by the tie-breakers the terms leave out. Passing its own result
// back returns the same order, so a backend can call it whether or not the query service already
// settled the terms.
func EffectiveSpanOrder(order []SpanSortOrder) ([]SpanSortOrder, error) {
	out, err := NormalizeSpanOrder(order)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(out))
	for _, term := range out {
		seen[term.Expression.(*expression.FieldRef).Name] = true
	}
	for _, field := range []string{"startTime", "traceID", "spanID"} {
		if seen[field] {
			continue
		}
		direction := SortAscending
		if field == "startTime" {
			direction = SortDescending
		}
		out = append(out, SpanSortOrder{
			Expression: &expression.FieldRef{Level: expression.LevelSpan, Name: field},
			Direction:  direction,
		})
	}
	return out, nil
}

// ValidateSpanSorting refuses explicit ordering unless the reader supports the complete contract.
func (c SearchCapabilities) ValidateSpanSorting(order []SpanSortOrder) error {
	if len(order) > 0 && (!c.SpanSearch || !c.SpanSorting) {
		return fmt.Errorf("%w: this storage backend does not support explicit span ordering", ErrSpanOrderUnsupported)
	}
	return nil
}
