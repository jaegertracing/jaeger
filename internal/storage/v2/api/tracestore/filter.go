// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"slices"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// ErrFilterUnsupported is returned for a well-formed query filter that the storage cannot
// serve — a level it does not index, an operator it has not implemented, or a boolean
// structure a flat index cannot evaluate (RFC 0005 §7). The query is refused rather than
// approximated, so a caller never reads a narrower answer as the whole one. The query
// service returns it for the limits a Reader declared through FilterCapabilities, and a
// Reader returns it for the ones that declaration is too coarse to express — a built-in
// field of a level it serves but does not store, or an operator it serves on some
// references and not others.
var ErrFilterUnsupported = errors.New("this storage backend cannot serve this query filter")

// ErrFilterInvalid is returned for a query filter whose value does not fit the field it
// compares — the kind of mistake a structural check cannot catch, because the filter AST
// deliberately does not carry types (RFC 0005 §6.1).
var ErrFilterInvalid = errors.New("invalid query filter")

// FilterCapabilities declares how much of a structured filter a Reader evaluates, by naming
// the levels and operators it serves. Nothing is implicit: a Reader opts in only by naming
// something, so an empty FilterCapabilities says the same as no FilterCapabilities at all —
// see IsEmpty, which is what callers ask rather than comparing against nil.
type FilterCapabilities struct {
	// Levels are the levels a Reference may name. Empty means the Reader can serve only
	// unqualified references.
	Levels []expression.Level
	// Operators are the operators the Reader evaluates. The boolean combinators are listed
	// here like any other operator: a flat inverted index declares OpAnd and omits OpOr and
	// OpNot, which is what confines it to the conjunctive subset. Nesting is not declared
	// separately, because OpAnd is associative and a caller flattens it before asking.
	Operators []expression.Operator
}

// IsEmpty reports whether the Reader declared no part of a structured filter, and so evaluates
// none of it. A nil FilterCapabilities and an empty one both answer true, so a caller never has
// to tell "declared nothing" from "did not declare", and a Reader cannot half-opt-in.
func (c *FilterCapabilities) IsEmpty() bool {
	return c == nil || (len(c.Levels) == 0 && len(c.Operators) == 0)
}

// SupportsLevel reports whether a Reference at the given level may reach the Reader.
// An unqualified reference always may.
func (c FilterCapabilities) SupportsLevel(level expression.Level) bool {
	if level == "" {
		return true
	}
	return slices.Contains(c.Levels, level)
}

// SupportsOperator reports whether the Reader evaluates the given operator.
func (c FilterCapabilities) SupportsOperator(op expression.Operator) bool {
	return slices.Contains(c.Operators, op)
}
