// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// orderedAttributePredicate is an attribute comparison that requires the
// typed numeric sub-field. Its operator is retained so a mapping-based refusal
// has the same useful error as a gate-based refusal.
type orderedAttributePredicate struct {
	op  expression.Operator
	ref reference
}

// validateOrderedAttributeMappings refuses an ordered attribute comparison if
// any concrete index in this search window can only index that attribute as
// text. A query over an old and a new index would otherwise silently skip the
// old index's values, which is a narrower result than the filter requested.
func (s *SpanReader) validateOrderedAttributeMappings(
	ctx context.Context,
	filter *expression.Call,
	indices []string,
) error {
	predicates := orderedAttributePredicates(filter)
	if len(predicates) == 0 {
		return nil
	}
	if s.getMappings == nil {
		return fmt.Errorf("ordered attribute mapping reader is not configured")
	}
	mappings, err := s.getMappings(ctx, indices)
	if err != nil {
		return fmt.Errorf("get span index mappings: %w", err)
	}
	for _, predicate := range predicates {
		for _, mapping := range mappings {
			if !s.mappingSupportsOrderedAttribute(mapping.Mappings, predicate.ref) {
				return errUnorderedValue(predicate.op, predicate.ref)
			}
		}
	}
	return nil
}

// orderedAttributePredicates finds range comparisons below boolean operators.
// buildFindTraceIDsQuery has already validated the tree before this helper is
// called, so malformed calls can be ignored here rather than reimplementing the
// filter builder's error handling.
func orderedAttributePredicates(filter *expression.Call) []orderedAttributePredicate {
	var predicates []orderedAttributePredicate
	var visit func(*expression.Call)
	visit = func(call *expression.Call) {
		if call == nil {
			return
		}
		if ordersValues(call.Op) {
			ref, _, err := refAndConstantArgs(call)
			if err == nil && ref.attribute {
				predicates = append(predicates, orderedAttributePredicate{op: call.Op, ref: ref})
			}
			return
		}
		for _, arg := range call.Args {
			if nested, ok := arg.(*expression.Call); ok {
				visit(nested)
			}
		}
	}
	visit(filter)
	return predicates
}

// mappingSupportsOrderedAttribute checks every representation that is present
// in an index. A missing dynamic object property cannot hold any documents, so
// it is safe to skip; a present property without value.number would make the
// range query omit documents and therefore must refuse the search.
func (s *SpanReader) mappingSupportsOrderedAttribute(mapping esclient.Mapping, ref reference) bool {
	locations, ok := attributeLocations[ref.level]
	if !ok {
		return false
	}
	for _, field := range locations.object {
		property, found := mapping.Property(s.objectAttributeField(field, ref.name))
		if found && !property.HasNumericSubfield() {
			return false
		}
	}
	for _, path := range locations.nested {
		property, found := mapping.Property(nestedField(path, tagValueField))
		if found && !property.HasNumericSubfield() {
			return false
		}
	}
	return true
}
