// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sqlquery

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AfterShip/clickhouse-sql-parser/parser"
)

// Error is a refusal of a statement, placed at the construct that caused it. The message says
// what is wrong and the hint says what the dialect admits instead, because for an agent the
// error text is the documentation at the moment it matters (RFC 0019 §5).
type Error struct {
	Line    int
	Column  int
	Message string
	Hint    string
}

func (e *Error) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "line %d:%d: %s", e.Line, e.Column, e.Message)
	if e.Hint != "" {
		b.WriteString(". ")
		b.WriteString(e.Hint)
	}
	return b.String()
}

// hintOneRelation is the hint for every construct that would read more than one relation.
const hintOneRelation = "A statement reads one relation, spans or traces; events and links are reached through EXISTS (SELECT 1 FROM events e WHERE e.span_id = s.span_id AND ...)"

// hintWindow shows the two conjuncts every search needs.
const hintWindow = "Add both bounds to WHERE, for example between(start_time, now() - '1h', now())"

// hintAggregation names where the aggregation clauses are reserved.
const hintAggregation = "GROUP BY, HAVING and aggregate functions are reserved for the row-returning RPC of RFC 0016 §5 and are not supported yet"

// positions maps a byte offset in the statement to a line and column, both 1-based.
type positions struct {
	sql        string
	lineStarts []int
}

func newPositions(sql string) positions {
	starts := []int{0}
	for i, c := range sql {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return positions{sql: sql, lineStarts: starts}
}

// at places an error at the start of a node. A string literal's position is that of its text,
// so the opening quote is one byte before it.
func (p positions) at(node parser.Expr, message, hint string) *Error {
	offset := int(node.Pos())
	if _, ok := node.(*parser.StringLiteral); ok {
		offset--
	}
	return p.atOffset(offset, message, hint)
}

// atOffset places an error at a byte offset of the statement, counted from 0 as the parser
// counts them.
func (p positions) atOffset(offset int, message, hint string) *Error {
	offset = max(0, min(offset, len(p.sql)))
	line := 1
	for line < len(p.lineStarts) && p.lineStarts[line] <= offset {
		line++
	}
	return &Error{
		Line:    line,
		Column:  offset - p.lineStarts[line-1] + 1,
		Message: message,
		Hint:    hint,
	}
}

// fromParser converts the parser's own error, keeping its position and dropping the caret
// rendering, which is meant for a terminal.
func (p positions) fromParser(err error) error {
	const hint = "The statement did not parse as SQL; the dialect is a read-only SELECT over spans or traces (RFC 0019 §4)"
	var parseErr *parser.ParseError
	if !errors.As(err, &parseErr) {
		return p.atOffset(0, err.Error(), hint)
	}
	first, _, _ := strings.Cut(parseErr.Error(), "\n")
	first = strings.TrimPrefix(first, fmt.Sprintf("line %d:%d ", parseErr.Line, parseErr.Column))
	return p.atOffset(int(parseErr.Pos), first, hint)
}
