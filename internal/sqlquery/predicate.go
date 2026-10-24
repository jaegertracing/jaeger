// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sqlquery

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AfterShip/clickhouse-sql-parser/parser"

	ast "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/expression"
)

// builder lowers to the same tree a Go caller would build by hand (RFC 0019 §5).
var builder expression.Predicate

// scope is the relation a predicate's column names resolve against: the one alias of the
// statement, or the events or links alias of an EXISTS subquery, which also sees the outer
// alias for its correlation.
type scope struct {
	alias string
	rel   relation
	outer *scope
}

// resolved is a column reference read against a scope. outer is true when the reference belongs
// to the outer scope, which only the correlation of an EXISTS subquery is allowed to do; a
// correlation column of events or links resolves to the zero column.
type resolved struct {
	column
	name  string
	outer bool
}

// resolveColumn reads a column reference, qualified by an alias or not, against the scope.
func (c *compiler) resolveColumn(sc *scope, node parser.Expr) (resolved, error) {
	var qualifier, name string
	switch ref := node.(type) {
	case *parser.Ident:
		name = ref.Name
	case *parser.Path:
		if len(ref.Fields) != 2 {
			return resolved{}, c.pos.at(node, "a column is written as alias.column or column", "")
		}
		qualifier, name = ref.Fields[0].Name, ref.Fields[1].Name
	default:
		return resolved{}, c.pos.at(node, "expected a column name here", "A predicate compares a column with a literal, another column, or a list")
	}
	target, outer := sc, false
	if qualifier != "" && !strings.EqualFold(qualifier, sc.alias) {
		if sc.outer == nil || !strings.EqualFold(qualifier, sc.outer.alias) {
			return resolved{}, c.pos.at(node, fmt.Sprintf("unknown alias %q", qualifier), fmt.Sprintf("The statement reads %s as %s", sc.rel, sc.alias))
		}
		target, outer = sc.outer, true
	}
	col, ok := columnsOf(target.rel)[columnKey(name)]
	if !ok {
		if isCorrelationColumn(name) && target.rel != relationSpans {
			return resolved{name: name, outer: outer}, nil
		}
		return resolved{}, c.pos.at(node, fmt.Sprintf("%s has no column %q", target.rel, name), "Its columns are "+columnNames(target.rel))
	}
	return resolved{column: col, name: name, outer: outer}, nil
}

// reference lowers a column or a map subscript to the term a predicate names. It refuses a
// reference to the outer alias, because inside EXISTS only the correlation may name it.
func (c *compiler) reference(sc *scope, node parser.Expr) (ast.Expression, error) {
	if fn, ok := node.(*parser.FunctionExpr); ok && strings.EqualFold(fn.Name.Name, "attr") {
		args := functionArgs(fn)
		if len(args) != 1 {
			return nil, c.pos.at(node, "attr() takes one argument, the attribute key", "")
		}
		key, ok := args[0].(*parser.StringLiteral)
		if !ok {
			return nil, c.pos.at(args[0], "the argument of attr() is a quoted attribute key", "")
		}
		return &ast.AttributeRef{Key: unquote(key.Literal)}, nil
	}
	if sub, ok := node.(*parser.ObjectParams); ok {
		col, err := c.resolveColumn(sc, sub.Object)
		if err != nil {
			return nil, err
		}
		if col.outer {
			return nil, c.outerReference(node, sc)
		}
		if !col.isMap() {
			return nil, c.pos.at(node, fmt.Sprintf("%q is not a map column and takes no subscript", col.name), "The map columns are attributes, resource_attributes and scope_attributes")
		}
		if sub.Params == nil || sub.Params.Items == nil || len(sub.Params.Items.Items) != 1 {
			return nil, c.pos.at(node, "a map subscript names one attribute key", "")
		}
		key, ok := unwrapColumnExpr(sub.Params.Items.Items[0]).(*parser.StringLiteral)
		if !ok {
			return nil, c.pos.at(sub.Params, "the subscript of a map column is a quoted attribute key", "")
		}
		return &ast.AttributeRef{Key: unquote(key.Literal), Level: col.Level}, nil
	}
	col, err := c.resolveColumn(sc, node)
	if err != nil {
		return nil, err
	}
	if col.outer {
		return nil, c.outerReference(node, sc)
	}
	if col.Level == "" {
		return nil, c.pos.at(node, fmt.Sprintf("%q is a correlation column, not a predicate", col.name), "Inside EXISTS write e.span_id = s.span_id once; it ties the event to its span")
	}
	if col.isMap() {
		return nil, c.pos.at(node, fmt.Sprintf("%q is a map and is read by subscript", col.name), fmt.Sprintf("Write %s['key'], or attr('key') to search the span and resource levels together", col.name))
	}
	return &ast.FieldRef{Name: col.Field, Level: col.Level}, nil
}

func (c *compiler) outerReference(node parser.Expr, sc *scope) error {
	return c.pos.at(node, fmt.Sprintf("a predicate inside EXISTS reads the %s columns only", sc.rel), "The outer alias appears once, in the correlation e.span_id = s.span_id")
}

// isReference reports whether a node names a value on the span rather than a constant.
func isReference(node parser.Expr) bool {
	switch n := node.(type) {
	case *parser.Ident:
		return !isBoolean(n.Name)
	case *parser.Path, *parser.ObjectParams:
		return true
	case *parser.FunctionExpr:
		return strings.EqualFold(n.Name.Name, "attr")
	}
	return false
}

func isBoolean(name string) bool {
	return strings.EqualFold(name, "true") || strings.EqualFold(name, "false")
}

// constant lowers a literal to the untyped constant the wire carries. The field beside it types
// it (RFC 0005 §5.4), so the only rewriting here is of SQL spellings the IDL does not have: an
// INTERVAL into Go duration syntax, and now() arithmetic into an RFC 3339 instant.
func (c *compiler) constant(node parser.Expr) (ast.Expression, error) {
	text, err := c.constantText(node)
	if err != nil {
		return nil, err
	}
	return &ast.AnyValue{Value: text}, nil
}

func (c *compiler) constantText(node parser.Expr) (string, error) {
	switch n := node.(type) {
	case *parser.StringLiteral:
		return unquote(n.Literal), nil
	case *parser.NumberLiteral:
		return n.Literal, nil
	case *parser.Ident:
		if isBoolean(n.Name) {
			return strings.ToLower(n.Name), nil
		}
	case *parser.IntervalExpr:
		d, err := c.interval(n)
		if err != nil {
			return "", err
		}
		return d.String(), nil
	case *parser.FunctionExpr, *parser.BinaryOperation:
		if t, ok, err := c.instant(node); ok || err != nil {
			if err != nil {
				return "", err
			}
			return t.Format(time.RFC3339Nano), nil
		}
	default:
	}
	return "", c.pos.at(node, "expected a literal here", "Literals are quoted strings, numbers, true or false, INTERVAL '2' SECOND, and now() with an INTERVAL added or subtracted")
}

// instant evaluates now() and now() ± INTERVAL, the only arithmetic the dialect has. ok is false
// for a node that is not that shape, so the caller can report it in its own words.
func (c *compiler) instant(node parser.Expr) (t time.Time, ok bool, err error) {
	switch n := node.(type) {
	case *parser.FunctionExpr:
		if !strings.EqualFold(n.Name.Name, "now") {
			return time.Time{}, false, nil
		}
		if len(functionArgs(n)) != 0 {
			return time.Time{}, true, c.pos.at(node, "now() takes no arguments", "")
		}
		return c.now, true, nil
	case *parser.BinaryOperation:
		if n.Operation != parser.TokenKindPlus && n.Operation != parser.TokenKindMinus {
			return time.Time{}, false, nil
		}
		base, ok, err := c.instant(n.LeftExpr)
		if err != nil || !ok {
			return time.Time{}, ok, err
		}
		interval, ok := n.RightExpr.(*parser.IntervalExpr)
		if !ok {
			return time.Time{}, true, c.pos.at(n.RightExpr, "only an INTERVAL may be added to or subtracted from now()", "")
		}
		d, err := c.interval(interval)
		if err != nil {
			return time.Time{}, true, err
		}
		if n.Operation == parser.TokenKindMinus {
			d = -d
		}
		return base.Add(d), true, nil
	}
	return time.Time{}, false, nil
}

var intervalUnits = map[string]time.Duration{
	"NANOSECOND": time.Nanosecond, "MICROSECOND": time.Microsecond, "MILLISECOND": time.Millisecond,
	"SECOND": time.Second, "MINUTE": time.Minute, "HOUR": time.Hour, "DAY": 24 * time.Hour, "WEEK": 7 * 24 * time.Hour,
}

func (c *compiler) interval(n *parser.IntervalExpr) (time.Duration, error) {
	var text string
	switch v := n.Expr.(type) {
	case *parser.StringLiteral:
		text = unquote(v.Literal)
	case *parser.NumberLiteral:
		text = v.Literal
	default:
		return 0, c.pos.at(n, "an INTERVAL takes a number, as INTERVAL '2' SECOND", "")
	}
	count, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, c.pos.at(n, fmt.Sprintf("%q is not a whole number", text), "An INTERVAL takes a whole number, as INTERVAL '90' SECOND")
	}
	if n.Unit == nil {
		return 0, c.pos.at(n, "an INTERVAL needs a unit", "The units are NANOSECOND, MICROSECOND, MILLISECOND, SECOND, MINUTE, HOUR, DAY and WEEK")
	}
	unit, ok := intervalUnits[strings.ToUpper(strings.TrimSuffix(strings.ToUpper(n.Unit.Name), "S"))]
	if !ok {
		return 0, c.pos.at(n.Unit, fmt.Sprintf("%q is not an interval unit the dialect knows", n.Unit.Name), "The units are NANOSECOND, MICROSECOND, MILLISECOND, SECOND, MINUTE, HOUR, DAY and WEEK; a month or a year has no fixed length")
	}
	return time.Duration(count) * unit, nil
}

// predicate lowers a boolean expression to an RFC 0005 call.
func (c *compiler) predicate(sc *scope, node parser.Expr) (*ast.Call, error) {
	switch n := node.(type) {
	case *parser.ParamExprList:
		inner, ok := parenthesized(n)
		if !ok {
			return nil, c.pos.at(node, "a parenthesized group holds one expression", "")
		}
		return c.predicate(sc, inner)
	case *parser.UnaryExpr:
		if !strings.EqualFold(string(n.Kind), parser.KeywordNot) {
			return nil, c.pos.at(node, fmt.Sprintf("the operator %s is not supported", n.Kind), "")
		}
		inner, err := c.predicate(sc, n.Expr)
		if err != nil {
			return nil, err
		}
		return builder.Not(inner), nil
	case *parser.IsNullExpr:
		ref, err := c.attributeReference(sc, n.Expr, "IS NULL")
		if err != nil {
			return nil, err
		}
		return builder.Not(ref.Exists()), nil
	case *parser.IsNotNullExpr:
		ref, err := c.attributeReference(sc, n.Expr, "IS NOT NULL")
		if err != nil {
			return nil, err
		}
		return ref.Exists(), nil
	case *parser.BetweenClause:
		return c.between(sc, n)
	case *parser.FunctionExpr:
		return c.functionPredicate(sc, n)
	case *parser.BinaryOperation:
		return c.binary(sc, n)
	}
	return nil, c.pos.at(node, "expected a predicate here", "A predicate is a comparison, IN, LIKE, REGEXP, IS [NOT] NULL, BETWEEN, EXISTS (SELECT ...), match(), phrase() or fulltext(), joined by AND, OR and NOT")
}

// attributeReference resolves the operand of IS [NOT] NULL, which applies to attributes only:
// a built-in field is always present (RFC 0019 §4.2).
func (c *compiler) attributeReference(sc *scope, node parser.Expr, op string) (expression.Ref, error) {
	ref, err := c.reference(sc, node)
	if err != nil {
		return expression.Ref{}, err
	}
	if _, ok := ref.(*ast.AttributeRef); !ok {
		return expression.Ref{}, c.pos.at(node, op+" applies to an attribute, and a built-in field is always present", "Write attributes['key'] IS NOT NULL")
	}
	return builder.RefOf(ref), nil
}

func (c *compiler) between(sc *scope, n *parser.BetweenClause) (*ast.Call, error) {
	ref, err := c.reference(sc, n.Expr)
	if err != nil {
		return nil, err
	}
	low, err := c.operand(sc, n.Between)
	if err != nil {
		return nil, err
	}
	high, err := c.operand(sc, n.And)
	if err != nil {
		return nil, err
	}
	r := builder.RefOf(ref)
	call := builder.And(r.Gte(low), r.Lte(high))
	if n.Not {
		return builder.Not(call), nil
	}
	return call, nil
}

// operand lowers the right-hand side of a comparison: a reference compares two values on the
// span, anything else is a constant.
func (c *compiler) operand(sc *scope, node parser.Expr) (ast.Expression, error) {
	if isReference(node) {
		return c.reference(sc, node)
	}
	return c.constant(node)
}

func (c *compiler) binary(sc *scope, n *parser.BinaryOperation) (*ast.Call, error) {
	op := strings.ToUpper(string(n.Operation))
	switch op {
	case parser.KeywordAnd, parser.KeywordOr:
		left, err := c.predicate(sc, n.LeftExpr)
		if err != nil {
			return nil, err
		}
		right, err := c.predicate(sc, n.RightExpr)
		if err != nil {
			return nil, err
		}
		if op == parser.KeywordAnd {
			return builder.And(left, right), nil
		}
		return builder.Or(left, right), nil
	}
	if !isReference(n.LeftExpr) {
		if isReference(n.RightExpr) && comparisonOps[op] != "" {
			// A constant on the left reads the same with the operands swapped and the order
			// reversed, which is the orientation the resolver puts every comparison in anyway.
			return c.binary(sc, &parser.BinaryOperation{LeftExpr: n.RightExpr, Operation: parser.TokenKind(mirror(op)), RightExpr: n.LeftExpr})
		}
		return nil, c.operandError(n.LeftExpr)
	}
	refExpr, err := c.reference(sc, n.LeftExpr)
	if err != nil {
		return nil, err
	}
	ref := builder.RefOf(refExpr)
	switch op {
	case "IN", "NOT IN":
		values, err := c.list(sc, n.RightExpr)
		if err != nil {
			return nil, err
		}
		if op == "IN" {
			return ref.In(values...), nil
		}
		return ref.NotIn(values...), nil
	case "LIKE", "NOT LIKE":
		pattern, ok := n.RightExpr.(*parser.StringLiteral)
		if !ok {
			return nil, c.pos.at(n.RightExpr, "the pattern of LIKE is a quoted string", "")
		}
		call, err := c.like(n.RightExpr, ref, unquote(pattern.Literal))
		if err != nil {
			return nil, err
		}
		if op == "NOT LIKE" {
			return builder.Not(call), nil
		}
		return call, nil
	case "ILIKE", "NOT ILIKE":
		return nil, c.pos.at(n, "ILIKE is not supported", "The regex operator of RFC 0005 matches case-sensitively, so a case-insensitive pattern has no target; write LIKE or REGEXP with the case the value has")
	case "REGEXP":
		pattern, ok := n.RightExpr.(*parser.StringLiteral)
		if !ok {
			return nil, c.pos.at(n.RightExpr, "the pattern of REGEXP is a quoted string", "")
		}
		return ref.Matches(unquote(pattern.Literal)), nil
	}
	if astOp := comparisonOps[op]; astOp != "" {
		value, err := c.operand(sc, n.RightExpr)
		if err != nil {
			return nil, err
		}
		return builder.Compare(astOp, ref, value), nil
	}
	return nil, c.pos.at(n, fmt.Sprintf("the operator %s is not supported", n.Operation), "The operators are =, <>, <, <=, >, >=, IN, NOT IN, LIKE, ILIKE, REGEXP, IS NULL, IS NOT NULL and BETWEEN")
}

// operandError says why the left operand of a comparison cannot be lowered, naming the construct
// where there is one: an aggregate or a function the dialect lacks, or arithmetic.
func (c *compiler) operandError(node parser.Expr) error {
	switch n := node.(type) {
	case *parser.FunctionExpr:
		return c.functionError(n)
	case *parser.BinaryOperation:
		return c.pos.at(n, fmt.Sprintf("the operator %s is not supported", n.Operation), "Arithmetic has no target in the filter; compare the column with a literal")
	case *parser.UnaryExpr:
		return c.pos.at(n, fmt.Sprintf("the operator %s is not supported", n.Kind), "Arithmetic has no target in the filter; compare the column with a literal")
	}
	return c.pos.at(node, "a comparison names a column on at least one side", "Columns are written as alias.column, column, attributes['key'] or attr('key')")
}

// functionError refuses a function that is not one of the dialect's.
func (c *compiler) functionError(n *parser.FunctionExpr) error {
	name := strings.ToLower(n.Name.Name)
	if isAggregate(name) {
		return c.pos.at(n, name+"() is an aggregate", hintAggregation)
	}
	return c.pos.at(n, fmt.Sprintf("the function %s() is not supported", n.Name.Name), "The functions are match(column, 'regex'), phrase(column, 'words'), fulltext(column, 'words'), attr('key') and now()")
}

var comparisonOps = map[string]ast.Operator{
	"=": ast.OpEq, "==": ast.OpEq, "!=": ast.OpNe, "<>": ast.OpNe,
	">": ast.OpGt, "<": ast.OpLt, ">=": ast.OpGte, "<=": ast.OpLte,
}

// mirror returns the operator that reads the same with its operands swapped.
func mirror(op string) string {
	switch op {
	case ">":
		return "<"
	case "<":
		return ">"
	case ">=":
		return "<="
	case "<=":
		return ">="
	}
	return op
}

// list lowers the right-hand side of IN, a parenthesized list of literals. A subquery there is
// refused: the one admitted subquery shape, trace_id IN (SELECT ...), is a statement shape that
// compileTraces consumes before any predicate is lowered (RFC 0019 §4.4).
func (c *compiler) list(_ *scope, node parser.Expr) ([]any, error) {
	if _, ok := node.(*parser.SubQuery); ok {
		return nil, c.pos.at(node, "a subquery in IN is accepted only as SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE ...)", hintOneRelation)
	}
	params, ok := node.(*parser.ParamExprList)
	if !ok || params.Items == nil {
		return nil, c.pos.at(node, "IN takes a parenthesized list of literals", "")
	}
	values := make([]any, 0, len(params.Items.Items))
	for _, item := range params.Items.Items {
		text, err := c.constantText(unwrapColumnExpr(item))
		if err != nil {
			return nil, err
		}
		values = append(values, text)
	}
	return values, nil
}

// functionPredicate lowers the functions that stand in for operators SQL does not have: match()
// for regex, phrase() and fulltext() for text search, and EXISTS for the quantifier over events
// or links.
func (c *compiler) functionPredicate(sc *scope, n *parser.FunctionExpr) (*ast.Call, error) {
	name := strings.ToLower(n.Name.Name)
	args := functionArgs(n)
	switch name {
	case "exists":
		if len(args) != 1 {
			return nil, c.pos.at(n, "EXISTS takes one subquery", hintOneRelation)
		}
		sub, ok := args[0].(*parser.SelectQuery)
		if !ok {
			return nil, c.pos.at(args[0], "EXISTS takes a subquery", hintOneRelation)
		}
		return c.exists(sc, sub)
	case "match", "phrase", "fulltext":
		if len(args) < 2 {
			return nil, c.pos.at(n, name+"() takes a column and a quoted pattern", "")
		}
		refExpr, err := c.reference(sc, args[0])
		if err != nil {
			return nil, err
		}
		ref := builder.RefOf(refExpr)
		var words []string
		for _, arg := range args[1:] {
			lit, ok := arg.(*parser.StringLiteral)
			if !ok {
				return nil, c.pos.at(arg, "the pattern of "+name+"() is a quoted string", "")
			}
			words = append(words, strings.Fields(unquote(lit.Literal))...)
		}
		switch name {
		case "match":
			if len(args) != 2 {
				return nil, c.pos.at(n, "match() takes a column and one pattern", "")
			}
			return ref.Matches(unquote(args[1].(*parser.StringLiteral).Literal)), nil
		case "phrase":
			return ref.Phrase(words...), nil
		default:
			return ref.Fulltext(words...), nil
		}
	default:
		return nil, c.functionError(n)
	}
}

func isAggregate(name string) bool {
	switch name {
	case "count", "sum", "avg", "min", "max", "quantile", "median", "any", "uniq":
		return true
	}
	return false
}

// exists lowers the correlated subquery of RFC 0019 §4.3 to some(events, ...) or some(links, ...).
func (c *compiler) exists(outer *scope, sub *parser.SelectQuery) (*ast.Call, error) {
	if err := c.refuseClauses(sub); err != nil {
		return nil, err
	}
	if sub.OrderBy != nil || sub.Limit != nil {
		return nil, c.pos.at(sub, "an EXISTS subquery takes no ORDER BY or LIMIT", "")
	}
	rel, alias, err := c.relationOf(sub)
	if err != nil {
		return nil, err
	}
	if rel != relationEvents && rel != relationLinks {
		return nil, c.pos.at(sub.From, "EXISTS reads events or links", hintOneRelation)
	}
	if outer.rel != relationSpans || outer.outer != nil {
		return nil, c.pos.at(sub.From, "EXISTS appears in the WHERE of a spans query", hintOneRelation)
	}
	if sub.Where == nil {
		return nil, c.pos.at(sub, "an EXISTS subquery needs WHERE "+alias+".span_id = "+outer.alias+".span_id and a predicate", hintOneRelation)
	}
	sc := &scope{alias: alias, rel: rel, outer: outer}
	var predicates []*ast.Call
	correlated := false
	for _, conjunct := range conjuncts(sub.Where.Expr) {
		if isCorrelation, err := c.correlation(sc, conjunct); err != nil {
			return nil, err
		} else if isCorrelation {
			correlated = true
			continue
		}
		pred, err := c.predicate(sc, conjunct)
		if err != nil {
			return nil, err
		}
		predicates = append(predicates, pred)
	}
	if !correlated {
		return nil, c.pos.at(sub.Where, "an EXISTS subquery is tied to its span by "+alias+".span_id = "+outer.alias+".span_id", "Without the correlation the subquery would read every span's "+string(rel))
	}
	if len(predicates) == 0 {
		return nil, c.pos.at(sub.Where, "an EXISTS subquery needs a predicate on "+string(rel)+" beside the correlation", "For example "+alias+".name = 'exception'")
	}
	var collection expression.Collection = builder.Event()
	if rel == relationLinks {
		collection = builder.Link()
	}
	return builder.Some(collection, builder.And(predicates...)), nil
}

// correlation recognizes e.span_id = s.span_id and e.trace_id = s.trace_id, in either order,
// and refuses any other use of the correlation columns.
func (c *compiler) correlation(sc *scope, node parser.Expr) (bool, error) {
	bin, ok := node.(*parser.BinaryOperation)
	if !ok || string(bin.Operation) != "=" {
		return false, nil
	}
	left, right := bin.LeftExpr, bin.RightExpr
	if !isPlainColumn(left) || !isPlainColumn(right) {
		return false, nil
	}
	lhs, err := c.resolveColumn(sc, left)
	if err != nil {
		return false, nil //nolint:nilerr // Not a correlation; the predicate path reports the column.
	}
	rhs, err := c.resolveColumn(sc, right)
	if err != nil {
		return false, nil //nolint:nilerr // Same as above.
	}
	if !isCorrelationColumn(lhs.name) || !isCorrelationColumn(rhs.name) {
		return false, nil
	}
	if lhs.outer == rhs.outer || columnKey(lhs.name) != columnKey(rhs.name) {
		return false, c.pos.at(node, "a correlation ties the same column of the subquery and the outer alias", "Write "+sc.alias+".span_id = "+sc.outer.alias+".span_id")
	}
	return true, nil
}

func isPlainColumn(node parser.Expr) bool {
	switch node.(type) {
	case *parser.Ident, *parser.Path:
		return true
	}
	return false
}

// conjuncts flattens the top-level AND of a WHERE, looking through parentheses.
func conjuncts(node parser.Expr) []parser.Expr {
	if params, ok := node.(*parser.ParamExprList); ok {
		if inner, ok := parenthesized(params); ok {
			return conjuncts(inner)
		}
	}
	if bin, ok := node.(*parser.BinaryOperation); ok && strings.EqualFold(string(bin.Operation), parser.KeywordAnd) {
		return append(conjuncts(bin.LeftExpr), conjuncts(bin.RightExpr)...)
	}
	return []parser.Expr{node}
}

// parenthesized returns the one expression a parenthesized group holds.
func parenthesized(params *parser.ParamExprList) (parser.Expr, bool) {
	if params.Items == nil || len(params.Items.Items) != 1 {
		return nil, false
	}
	return unwrapColumnExpr(params.Items.Items[0]), true
}

func unwrapColumnExpr(node parser.Expr) parser.Expr {
	if col, ok := node.(*parser.ColumnExpr); ok {
		return col.Expr
	}
	return node
}

func functionArgs(fn *parser.FunctionExpr) []parser.Expr {
	if fn.Params == nil || fn.Params.Items == nil {
		return nil
	}
	args := make([]parser.Expr, 0, len(fn.Params.Items.Items))
	for _, item := range fn.Params.Items.Items {
		args = append(args, unwrapColumnExpr(item))
	}
	return args
}

// unquote reads the text of a string literal as the lexer slices it, between the quotes with
// the escapes still in place.
func unquote(literal string) string {
	var b strings.Builder
	for i := 0; i < len(literal); i++ {
		c := literal[i]
		switch {
		case c == '\'' && i+1 < len(literal) && literal[i+1] == '\'':
			b.WriteByte('\'')
			i++
		case c == '\\' && i+1 < len(literal):
			b.WriteByte(literal[i+1])
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// like lowers a LIKE pattern. The regex operator of RFC 0005 matches anywhere in the value and
// cannot be anchored, so only the patterns that mean the same thing have a target: one without
// a wildcard is an equality, and one with % at both ends is a regex over the text between them.
// A pattern anchored at one end, LIKE 'GET%', is refused rather than silently widened.
func (c *compiler) like(node parser.Expr, ref expression.Ref, pattern string) (*ast.Call, error) {
	if !strings.ContainsAny(pattern, "%_") {
		return ref.Eq(pattern), nil
	}
	inner, open := strings.CutPrefix(pattern, "%")
	inner, closed := strings.CutSuffix(inner, "%")
	if !open || !closed {
		return nil, c.pos.at(node, fmt.Sprintf("LIKE %q is anchored at one end, which the regex operator of RFC 0005 cannot express", pattern), "It matches anywhere in the value: write LIKE '%text%' to search for text anywhere, or = 'text' for the whole value")
	}
	return ref.Matches(likeToRegex(inner)), nil
}

// likeToRegex translates the wildcards of a LIKE pattern into RE2.
func likeToRegex(pattern string) string {
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}
