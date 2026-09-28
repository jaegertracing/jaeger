// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"fmt"
	"slices"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

type SortDirection string

const (
	SortAscending  SortDirection = "asc"
	SortDescending SortDirection = "desc"
)

var ErrSpanOrderInvalid = errors.New("invalid span ordering")

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

// EffectiveSpanOrder copies the terms, defaults their directions, and appends missing tie-breakers.
// Execution support is validated separately by NormalizeSpanOrder.
func EffectiveSpanOrder(order []SpanSortOrder) []SpanSortOrder {
	out := slices.Clone(order)
	seen := make(map[string]bool, len(order))
	for i, term := range out {
		if term.Direction == "" {
			out[i].Direction = SortAscending
		}
		if ref, ok := term.Expression.(*expression.FieldRef); ok && ref != nil && ref.Level == expression.LevelSpan {
			seen[ref.Name] = true
		}
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
	return out
}

// ValidateSpanSorting refuses explicit ordering unless the reader supports the complete contract.
func (c SearchCapabilities) ValidateSpanSorting(order []SpanSortOrder) error {
	if len(order) > 0 && (!c.SpanSearch || !c.SpanSorting) {
		return fmt.Errorf("%w: this storage backend does not support explicit span ordering", ErrSpanOrderInvalid)
	}
	return nil
}
