// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sqlquery

import (
	"errors"
	"testing"

	"github.com/AfterShip/clickhouse-sql-parser/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorRendering(t *testing.T) {
	err := &Error{Line: 2, Column: 7, Message: "the operator ^ is not supported"}
	assert.Equal(t, "line 2:7: the operator ^ is not supported", err.Error())
	err.Hint = "Use REGEXP"
	assert.Equal(t, "line 2:7: the operator ^ is not supported. Use REGEXP", err.Error())
}

func TestPositionsAtOffset(t *testing.T) {
	sql := "SELECT 1\nFROM spans\nWHERE é = 'x'"
	pos := newPositions(sql)
	tests := []struct {
		name   string
		offset int
		line   int
		column int
	}{
		{name: "start of the statement", offset: 0, line: 1, column: 1},
		{name: "last byte of the first line", offset: 8, line: 1, column: 9},
		{name: "first byte of the second line", offset: 9, line: 2, column: 1},
		{name: "after a two-byte character, counted as one column", offset: len("SELECT 1\nFROM spans\nWHERE é "), line: 3, column: 9},
		{name: "negative offset is clamped to the start", offset: -5, line: 1, column: 1},
		{name: "offset past the end is clamped to the end", offset: len(sql) + 10, line: 3, column: 14},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pos.atOffset(tt.offset, "msg", "hint")
			assert.Equal(t, tt.line, err.Line)
			assert.Equal(t, tt.column, err.Column)
			assert.Equal(t, "msg", err.Message)
			assert.Equal(t, "hint", err.Hint)
		})
	}
}

func TestPositionsAt_StringLiteralOpensOneByteEarlier(t *testing.T) {
	sql := "SELECT 'abc'"
	stmts, err := parser.NewParser(sql).ParseStmts()
	require.NoError(t, err)
	lit := stmts[0].(*parser.SelectQuery).SelectItems[0].Expr
	require.IsType(t, &parser.StringLiteral{}, lit)

	pos := newPositions(sql)
	assert.Equal(t, 8, pos.at(lit, "m", "").Column, "the quote, not the text")
}

func TestLeftmost(t *testing.T) {
	// The leaf every wrapper descends to is the string literal 'a'; a literal
	// leaf is what makes the one-byte shift of at() observable.
	leaf := &parser.StringLiteral{Literal: "a"}
	tests := []struct {
		name string
		node parser.Expr
	}{
		{name: "a leaf is its own leftmost", node: leaf},
		{name: "binary operation", node: &parser.BinaryOperation{LeftExpr: leaf, RightExpr: &parser.NumberLiteral{Literal: "1"}}},
		{name: "between clause", node: &parser.BetweenClause{Expr: leaf}},
		{name: "is null", node: &parser.IsNullExpr{Expr: leaf}},
		{name: "is not null", node: &parser.IsNotNullExpr{Expr: leaf}},
		{name: "map subscript", node: &parser.ObjectParams{Object: leaf}},
		{name: "nested wrappers", node: &parser.IsNullExpr{Expr: &parser.BinaryOperation{LeftExpr: &parser.ObjectParams{Object: leaf}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Same(t, leaf, leftmost(tt.node))
		})
	}
}

func TestFromParser(t *testing.T) {
	pos := newPositions("SELECT 1\n  FROM spans WHERE")

	t.Run("parse error keeps its position and drops the caret rendering", func(t *testing.T) {
		_, parseErr := parser.NewParser("SELECT 1\n  FROM spans WHERE").ParseStmts()
		require.Error(t, parseErr)
		var want *parser.ParseError
		require.ErrorAs(t, parseErr, &want)

		var got *Error
		require.ErrorAs(t, pos.fromParser(parseErr), &got)
		assert.Equal(t, want.Line, got.Line)
		assert.Equal(t, want.Column, got.Column)
		assert.NotContains(t, got.Message, "\n")
		assert.NotContains(t, got.Message, "line ")
		assert.Contains(t, got.Hint, "did not parse as SQL")
	})

	t.Run("any other error is placed at the start", func(t *testing.T) {
		var got *Error
		require.ErrorAs(t, pos.fromParser(errors.New("boom")), &got)
		assert.Equal(t, 1, got.Line)
		assert.Equal(t, 1, got.Column)
		assert.Equal(t, "boom", got.Message)
		assert.Contains(t, got.Hint, "did not parse as SQL")
	})
}
