// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// TestFilterCapabilities_IsEmpty pins that declaring nothing and not declaring at all are one
// answer, so a Reader cannot half-opt-in and a caller need not tell them apart.
func TestFilterCapabilities_IsEmpty(t *testing.T) {
	var absent *FilterCapabilities
	assert.True(t, absent.IsEmpty())
	assert.True(t, (&FilterCapabilities{}).IsEmpty())
	assert.True(t, (&FilterCapabilities{Levels: []expression.Level{}, Operators: []expression.Operator{}}).IsEmpty())
	assert.False(t, (&FilterCapabilities{Operators: []expression.Operator{expression.OpEq}}).IsEmpty())
	assert.False(t, (&FilterCapabilities{Levels: []expression.Level{expression.LevelSpan}}).IsEmpty())
}

func TestFilterCapabilities_SupportsLevel(t *testing.T) {
	caps := FilterCapabilities{Levels: []expression.Level{expression.LevelSpan, expression.LevelResource}}
	assert.True(t, caps.SupportsLevel(""), "an unqualified reference always reaches the reader")
	assert.True(t, caps.SupportsLevel(expression.LevelSpan))
	assert.True(t, caps.SupportsLevel(expression.LevelResource))
	assert.False(t, caps.SupportsLevel(expression.LevelLink))
	assert.False(t, FilterCapabilities{}.SupportsLevel(expression.LevelSpan))
}

// TestFilterCapabilities_SupportsOperator pins that the boolean combinators are declared
// like any other operator: a reader confined to the conjunctive subset lists expression.OpAnd and omits
// expression.OpOr and expression.OpNot, and nothing is implicit — the zero value declares no operator at all.
func TestFilterCapabilities_SupportsOperator(t *testing.T) {
	flat := FilterCapabilities{Operators: []expression.Operator{expression.OpAnd, expression.OpEq, expression.OpGte}}
	assert.True(t, flat.SupportsOperator(expression.OpAnd))
	assert.True(t, flat.SupportsOperator(expression.OpEq))
	assert.True(t, flat.SupportsOperator(expression.OpGte))
	assert.False(t, flat.SupportsOperator(expression.OpRegex))
	assert.False(t, flat.SupportsOperator(expression.OpOr))
	assert.False(t, flat.SupportsOperator(expression.OpNot))

	full := FilterCapabilities{Operators: []expression.Operator{expression.OpAnd, expression.OpOr, expression.OpNot, expression.OpEq}}
	assert.True(t, full.SupportsOperator(expression.OpOr))
	assert.True(t, full.SupportsOperator(expression.OpNot))

	assert.False(t, FilterCapabilities{}.SupportsOperator(expression.OpAnd), "nothing is implicit")
	assert.False(t, FilterCapabilities{}.SupportsOperator(expression.OpEq))
}

// TestSearchCapabilities_FieldCount is a tripwire rather than a property. The decorators
// that wrap a Reader are tested by setting each of these fields on its own, so a field
// added here without extending those tables would leave the new capability's forwarding
// unproven. Update the count with those tables.
func TestSearchCapabilities_FieldCount(t *testing.T) {
	assert.Equal(t, 3, reflect.TypeOf(SearchCapabilities{}).NumField(),
		"extend the permutation tables in the tracestoremetrics and queryinterceptor "+
			"reader-decorator tests, then update this count")
}
