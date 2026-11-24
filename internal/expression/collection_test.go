// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"

	ast "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// TestSomeQuantifiesOverACollection pins which levels Some accepts. Only the event and link levels
// implement Collection, so quantifying over the span or the resource does not compile — the
// commented lines below are the point of the test, and the ones that remain prove the two that do.
//
//	p.Some(p.Span(), predicate)     // does not compile
//	p.Some(p.Resource(), predicate) // does not compile
//	p.Some(p.Scope(), predicate)    // does not compile
func TestSomeQuantifiesOverACollection(t *testing.T) {
	predicate := p.Event().Name.Eq("exception")

	events := p.Some(p.Event(), predicate)
	assert.Equal(t, &ast.NestedRef{Level: ast.LevelEvent}, events.Args[0])

	links := p.Some(p.Link(), p.Link().TraceID.Exists())
	assert.Equal(t, &ast.NestedRef{Level: ast.LevelLink}, links.Args[0])
}

// TestCollectionIsOnlyTheTwoCollections is the compile-time claim written as an assertion, so that
// moving nested() back onto the level the five share would fail here rather than silently widen
// what Some accepts.
func TestCollectionIsOnlyTheTwoCollections(t *testing.T) {
	var _ Collection = EventLevel{}
	var _ Collection = LinkLevel{}

	assert.NotImplements(t, (*Collection)(nil), SpanLevel{})
	assert.NotImplements(t, (*Collection)(nil), ResourceLevel{})
	assert.NotImplements(t, (*Collection)(nil), ScopeLevel{})
}
