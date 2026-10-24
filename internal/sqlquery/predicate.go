// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sqlquery

import (
	"fmt"
	"math"
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
		if strings.EqualFold(ref.Name, "null") {
			return resolved{}, c.pos.at(node, "NULL is not a value to compare with", "Write attributes['key'] IS NULL or IS NOT NULL")
		}
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
		if col.Level == "" || !col.isMap() {
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
		return nil, c.pos.at(node, fmt.Sprintf("%q is a correlation column, not a predicate", col.name), "Inside EXISTS the collection is already the span's own; the column is accepted only as "+correlationExample(sc))
	}
	if col.isMap() {
		return nil, c.pos.at(node, fmt.Sprintf("%q is a map and is read by subscript", col.name), fmt.Sprintf("Write %s['key'], or attr('key') to search the span and resource levels together", col.name))
	}
	return &ast.FieldRef{Name: col.Field, Level: col.Level}, nil
}

func (c *compiler) outerReference(node parser.Expr, sc *scope) error {
	return c.pos.at(node, fmt.Sprintf("a predicate inside EXISTS reads the %s columns only", sc.rel), "The outer alias may appear only in the correlation "+correlationExample(sc)+", which is optional")
}

// correlationExample spells the optional correlation of an EXISTS subquery with its aliases.
func correlationExample(sc *scope) string {
	outer := "spans"
	if sc.outer != nil {
		outer = sc.outer.alias
	}
	return sc.alias + ".span_id = " + outer + ".span_id"
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
	return "", c.pos.at(node, "expected a literal here", "Literals are quoted strings, numbers, true or false, a duration such as '2s', and now() with a duration added or subtracted")
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
		d, err := c.offset(n.RightExpr)
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

// offset reads the duration added to or subtracted from now(): a string in the Go duration
// syntax every duration constant uses, '1h', or the SQL-standard INTERVAL '1' HOUR.
func (c *compiler) offset(node parser.Expr) (time.Duration, error) {
	switch n := unparenthesized(node).(type) {
	case *parser.StringLiteral:
		d, err := time.ParseDuration(unquote(n.Literal))
		if err != nil {
			return 0, c.pos.at(node, fmt.Sprintf("%q is not a duration", unquote(n.Literal)), "Write the offset from now() as '1h', '30m' or '90s'")
		}
		return d, nil
	case *parser.IntervalExpr:
		return c.interval(n)
	default:
		return 0, c.pos.at(node, "only a duration may be added to or subtracted from now()", "Write now() - '1h', or now() - INTERVAL '1' HOUR")
	}
}

// intervalUnits are the units of INTERVAL that have a fixed length; a month, a quarter or a year
// does not. The parser knows no finer unit than a millisecond and no plural spelling.
var intervalUnits = map[string]time.Duration{
	"MILLISECOND": time.Millisecond, "SECOND": time.Second, "MINUTE": time.Minute,
	"HOUR": time.Hour, "DAY": 24 * time.Hour, "WEEK": 7 * 24 * time.Hour,
}

const intervalUnitsHint = "The units are MILLISECOND, SECOND, MINUTE, HOUR, DAY and WEEK; a month or a year has no fixed length"

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
	unit, ok := intervalUnits[strings.ToUpper(n.Unit.Name)]
	if !ok {
		return 0, c.pos.at(n.Unit, fmt.Sprintf("%q is not an interval unit the dialect knows", n.Unit.Name), intervalUnitsHint)
	}
	if count > int64(math.MaxInt64/unit) || count < int64(math.MinInt64/unit) {
		return 0, c.pos.at(n, fmt.Sprintf("INTERVAL '%d' %s is longer than a duration can hold", count, n.Unit.Name), "The longest duration is about 292 years")
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
		r, _ := asRange(n)
		return c.between(sc, r)
	case *parser.FunctionExpr:
		if r, ok := asRange(n); ok {
			return c.between(sc, r)
		}
		return c.functionPredicate(sc, n)
	case *parser.BinaryOperation:
		return c.binary(sc, n)
	}
	return nil, c.pos.at(node, "expected a predicate here", "A predicate is a comparison, IN, LIKE, REGEXP, IS [NOT] NULL, between(), EXISTS (SELECT ...), match(), phrase() or fulltext(), joined by AND, OR and NOT")
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

// rangeTest is a BETWEEN in either of its spellings: the SQL operator x BETWEEN a AND b, or
// the function between(x, a, b), which reads better beside other conjuncts because it has no
// AND of its own. Both are inclusive at both ends.
type rangeTest struct {
	subject   parser.Expr
	low, high parser.Expr
	negated   bool
}

// asRange recognizes both spellings of BETWEEN, or reports that the node is neither.
func asRange(node parser.Expr) (rangeTest, bool) {
	switch n := node.(type) {
	case *parser.BetweenClause:
		return rangeTest{subject: n.Expr, low: n.Between, high: n.And, negated: n.Not}, true
	case *parser.FunctionExpr:
		if args := functionArgs(n); strings.EqualFold(n.Name.Name, "between") && len(args) == 3 {
			return rangeTest{subject: args[0], low: args[1], high: args[2]}, true
		}
	default:
	}
	return rangeTest{}, false
}

func (c *compiler) between(sc *scope, r rangeTest) (*ast.Call, error) {
	ref, err := c.reference(sc, r.subject)
	if err != nil {
		return nil, err
	}
	low, err := c.operand(sc, r.low)
	if err != nil {
		return nil, err
	}
	high, err := c.operand(sc, r.high)
	if err != nil {
		return nil, err
	}
	ro := builder.RefOf(ref)
	call := builder.And(ro.Gte(low), ro.Lte(high))
	if r.negated {
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
	if n.HasGlobal {
		return nil, c.pos.at(n, "GLOBAL IN is not supported", "Write IN or NOT IN; the search is not distributed")
	}
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
		values, err := c.list(n.RightExpr)
		if err != nil {
			return nil, err
		}
		if op == "IN" {
			return ref.In(values...), nil
		}
		return ref.NotIn(values...), nil
	case "LIKE", "NOT LIKE", "ILIKE", "NOT ILIKE":
		pattern, ok := n.RightExpr.(*parser.StringLiteral)
		if !ok {
			return nil, c.pos.at(n.RightExpr, "the pattern of LIKE is a quoted string", "")
		}
		call := c.like(ref, unquote(pattern.Literal), strings.Contains(op, "ILIKE"))
		if strings.HasPrefix(op, "NOT") {
			return builder.Not(call), nil
		}
		return call, nil
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
	switch n := unparenthesized(node).(type) {
	case *parser.FunctionExpr:
		if strings.EqualFold(n.Name.Name, "now") {
			break
		}
		return c.functionError(n)
	case *parser.BinaryOperation:
		if comparisonOps[strings.ToUpper(string(n.Operation))] != "" {
			return c.pos.at(n, "a comparison cannot be the operand of another comparison", "Join predicates with AND and OR")
		}
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
	return c.pos.at(n, fmt.Sprintf("the function %s() is not supported", n.Name.Name), "The functions are between(column, low, high), match(column, 'regex'), phrase(column, 'words'), fulltext(column, 'words'), attr('key') and now()")
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
// traces consumes before any predicate is lowered (RFC 0019 §4.4).
func (c *compiler) list(node parser.Expr) ([]any, error) {
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
	case "between":
		return nil, c.pos.at(n, "between() takes a column and two bounds", "Write between(duration, '1s', '2s')")
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

// exists lowers the subquery of RFC 0019 §4.3 to some(events, ...) or some(links, ...). Inside
// EXISTS the relation is the span's own collection, so a correlation to the outer alias,
// e.span_id = s.span_id, is accepted for the SQL reader's sake and adds nothing.
func (c *compiler) exists(outer *scope, sub *parser.SelectQuery) (*ast.Call, error) {
	if err := c.refuseClauses(sub); err != nil {
		return nil, err
	}
	if sub.OrderBy != nil || sub.Limit != nil {
		return nil, c.pos.at(sub, "an EXISTS subquery takes no ORDER BY or LIMIT", "")
	}
	if sub.HasDistinct || len(sub.SelectItems) != 1 || !isSelectOne(sub.SelectItems[0]) {
		return nil, c.pos.at(sub, "an EXISTS subquery is SELECT 1", "EXISTS asks whether a row exists; its select list has no effect")
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
	hint := "Inside EXISTS, " + string(rel) + " is the span's own collection; write a predicate on it, for example " + alias + ".name = 'exception'"
	if sub.Where == nil {
		return nil, c.pos.at(sub, "an EXISTS subquery needs a WHERE", hint)
	}
	sc := &scope{alias: alias, rel: rel, outer: outer}
	var predicates []*ast.Call
	for _, conjunct := range conjuncts(sub.Where.Expr) {
		if isCorrelation, err := c.correlation(sc, conjunct); err != nil {
			return nil, err
		} else if isCorrelation {
			continue
		}
		pred, err := c.predicate(sc, conjunct)
		if err != nil {
			return nil, err
		}
		predicates = append(predicates, pred)
	}
	if len(predicates) == 0 {
		return nil, c.pos.at(sub.Where, "an EXISTS subquery needs a predicate on "+string(rel), hint)
	}
	var collection expression.Collection = builder.Event()
	if rel == relationLinks {
		collection = builder.Link()
	}
	return builder.Some(collection, builder.And(predicates...)), nil
}

// correlation recognizes e.span_id = s.span_id and e.trace_id = s.trace_id, in either order, so
// that exists can drop them, and refuses any other use of the correlation columns.
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
// the escapes still in place. The quote is written ” or \', and a backslash is written \\.
// every other backslash is a character of the text, so a regex is written as it is read,
// '\d+', and a LIKE pattern escapes its wildcards as '\%' (RFC 0019 §4.2).
func unquote(literal string) string {
	var b strings.Builder
	for i := 0; i < len(literal); i++ {
		c := literal[i]
		switch {
		case c == '\'' && i+1 < len(literal) && literal[i+1] == '\'':
			b.WriteByte('\'')
			i++
		case c == '\\' && i+1 < len(literal) && (literal[i+1] == '\'' || literal[i+1] == '\\'):
			b.WriteByte(literal[i+1])
			i++
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// unparenthesized looks through the parentheses around one expression.
func unparenthesized(node parser.Expr) parser.Expr {
	for {
		params, ok := node.(*parser.ParamExprList)
		if !ok {
			return node
		}
		inner, ok := parenthesized(params)
		if !ok {
			return node
		}
		node = inner
	}
}

// isSelectOne reports whether a select item is the literal 1 of EXISTS (SELECT 1 ...).
func isSelectOne(item *parser.SelectItem) bool {
	lit, ok := item.Expr.(*parser.NumberLiteral)
	return ok && lit.Literal == "1" && item.Alias == nil && len(item.Modifiers) == 0
}

// like lowers a LIKE pattern to the anchored regular expression it means, or to an equality when
// it has no wildcard. ILIKE folds case with the (?i) flag, which keeps it a regex even without a
// wildcard. The anchors and the flag are honored by every backend (RFC 0005 §5.3).
func (*compiler) like(ref expression.Ref, pattern string, foldCase bool) *ast.Call {
	if !foldCase && !strings.ContainsAny(pattern, "%_") {
		return ref.Eq(likeLiteral(pattern))
	}
	regex := "^" + likeToRegex(pattern) + "$"
	if foldCase {
		regex = "(?i)" + regex
	}
	return ref.Matches(regex)
}

// likeToRegex translates the wildcards of a LIKE pattern into RE2. A backslash escapes the
// character after it, so '\%' and '\_' are the literal characters, as in ClickHouse. The
// wildcards are written as the class of every character, because SQL's match a newline and
// RE2's dot does not.
func likeToRegex(pattern string) string {
	var b strings.Builder
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch r := runes[i]; {
		case r == '\\' && i+1 < len(runes):
			i++
			b.WriteString(regexp.QuoteMeta(string(runes[i])))
		case r == '%':
			b.WriteString(`[\s\S]*`)
		case r == '_':
			b.WriteString(`[\s\S]`)
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}

// likeLiteral reads a LIKE pattern without wildcards as the text it matches, with its
// backslash escapes resolved.
func likeLiteral(pattern string) string {
	var b strings.Builder
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' && i+1 < len(runes) {
			i++
		}
		b.WriteRune(runes[i])
	}
	return b.String()
}
