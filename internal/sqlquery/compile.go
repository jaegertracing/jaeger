// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package sqlquery compiles the read-only SQL dialect of RFC 0019 into what the api_v3 RPCs
// take: the RFC 0005 filter, the start-time window, the RFC 0016 ordering and page size, and
// the RPC that the statement's shape selects.
//
// The package has no storage knowledge. Parsing is the job of the ClickHouse SQL parser
// library, whose dialect is standard SELECT grammar plus the map subscript and INTERVAL
// literal the dialect needs (RFC 0019 §6). The shape check and the lowering here consume that
// library's AST and emit Jaeger's, through the same internal/expression builder a Go caller
// uses, so a statement compiles to the tree the caller would have built by hand.
package sqlquery

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AfterShip/clickhouse-sql-parser/parser"

	ast "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Kind is the RPC a statement compiles to (RFC 0019 §4.4).
type Kind string

const (
	KindFindTraceIDs       Kind = "FindTraceIDs"
	KindFindSpans          Kind = "FindSpans"
	KindFindTraces         Kind = "FindTraces"
	KindFindTraceSummaries Kind = "FindTraceSummaries"
)

// Query is a compiled statement: the request the query service dispatches, and the RPC it
// dispatches it to.
type Query struct {
	Kind Kind
	// Filter is the RFC 0005 filter, finalized. It is nil when WHERE held only the window.
	Filter *ast.Call
	// StartTimeMin and StartTimeMax are the window extracted from WHERE; both are always set.
	StartTimeMin time.Time
	StartTimeMax time.Time
	// OrderBy is the span order of a FindSpans statement, normalized.
	OrderBy []tracestore.SpanSortOrder
	// Projection lists the columns a FindSpans statement selected, as references, or is nil
	// for SELECT *. It is reference-only, as RFC 0016 reserves the field.
	Projection []ast.Expression
	// Limit is the LIMIT of the statement, or 0 when absent. It is the page size of a span or
	// trace-ID search and the search depth of a trace search.
	Limit uint32
	// Normalized is the statement printed back in one canonical spelling, for logs and for the
	// dry run (RFC 0019 §5).
	Normalized string
}

// Compile parses and lowers one statement. now is the instant now() evaluates to, fixed once
// per request by the caller so that the two bounds of a window agree.
func Compile(sql string, now time.Time) (*Query, error) {
	c := &compiler{now: now, pos: newPositions(sql)}
	stmts, err := parser.NewParser(sql).ParseStmts()
	if err != nil {
		return nil, c.pos.fromParser(err)
	}
	if len(stmts) != 1 {
		return nil, c.pos.at(stmts[1], fmt.Sprintf("the request holds %d statements", len(stmts)), "Send one SELECT statement")
	}
	sel, ok := stmts[0].(*parser.SelectQuery)
	if !ok {
		return nil, c.pos.at(stmts[0], "only a SELECT statement is accepted", "The dialect is read-only (RFC 0019 §4)")
	}
	query, err := c.statement(sel)
	if err != nil {
		return nil, err
	}
	query.Normalized = formatted(sel)
	return query, nil
}

type compiler struct {
	now time.Time
	pos positions
}

func (c *compiler) statement(sel *parser.SelectQuery) (*Query, error) {
	if err := c.refuseClauses(sel); err != nil {
		return nil, err
	}
	rel, alias, err := c.relationOf(sel)
	if err != nil {
		return nil, err
	}
	switch rel {
	case relationSpans:
		return c.spans(sel, &scope{alias: alias, rel: rel})
	case relationTraces:
		return c.traces(sel, alias)
	default:
		return nil, c.pos.at(sel.From, string(rel)+" is read through EXISTS, not in FROM", hintOneRelation)
	}
}

// refuseClauses rejects every clause the dialect has no target for (RFC 0019 §4.5), with the
// reason, before the statement's shape is read.
func (c *compiler) refuseClauses(sel *parser.SelectQuery) error {
	switch {
	case sel.With != nil:
		return c.pos.at(sel.With, "WITH is not supported", hintOneRelation)
	case sel.InnerQuery != nil:
		return c.pos.at(sel, "a parenthesized query is not supported", "Write the SELECT without the outer parentheses")
	case sel.UnionAll != nil || sel.UnionDistinct != nil || sel.Except != nil || sel.Intersect != nil:
		return c.pos.at(sel, "UNION, EXCEPT and INTERSECT are not supported", "Combine the conditions with OR inside one WHERE instead")
	case sel.GroupBy != nil:
		return c.pos.at(sel.GroupBy, "GROUP BY is not supported yet", hintAggregation)
	case sel.Having != nil:
		return c.pos.at(sel.Having, "HAVING is not supported yet", hintAggregation)
	case sel.Window != nil:
		return c.pos.at(sel.Window, "window functions are not supported", "")
	case sel.Prewhere != nil:
		return c.pos.at(sel.Prewhere, "PREWHERE is not supported", "Put every condition in WHERE")
	case sel.LimitBy != nil:
		return c.pos.at(sel.LimitBy, "LIMIT BY is not supported", "")
	case sel.Settings != nil:
		return c.pos.at(sel.Settings, "SETTINGS is not supported", "")
	case sel.Format != nil:
		return c.pos.at(sel.Format, "FORMAT is not supported", "The response format is the RPC's")
	case sel.Top != nil:
		return c.pos.at(sel.Top, "TOP is not supported", "Write LIMIT n")
	case sel.DistinctOn != nil:
		return c.pos.at(sel.DistinctOn, "DISTINCT ON is not supported", "Write SELECT DISTINCT trace_id")
	case sel.Limit != nil && sel.Limit.Offset != nil:
		return c.pos.at(sel.Limit.Offset, "OFFSET is not supported", "Continue a search with the page token the previous response returned (RFC 0014)")
	}
	return nil
}

// relationOf reads FROM: one of the four relations, optionally aliased, and nothing else.
func (c *compiler) relationOf(sel *parser.SelectQuery) (relation, string, error) {
	if sel.From == nil {
		return "", "", c.pos.at(sel, "FROM is required", "Write FROM spans or FROM traces")
	}
	var table parser.Expr
	switch from := sel.From.Expr.(type) {
	case *parser.JoinExpr:
		return "", "", c.pos.at(from, "JOIN is not supported", hintOneRelation+". A self-join of spans would express RFC 0005's structural tier, which no backend serves")
	case *parser.JoinTableExpr:
		table = from.Table.Expr
	default:
		table = from
	}
	alias := ""
	if aliased, ok := table.(*parser.AliasExpr); ok {
		alias, table = formatted(aliased.Alias), aliased.Expr
	}
	ident, ok := table.(*parser.TableIdentifier)
	if !ok {
		return "", "", c.pos.at(table, "FROM takes a relation name", hintOneRelation+"; derived tables and table functions are not supported")
	}
	if ident.Database != nil {
		return "", "", c.pos.at(ident, "a relation has no database qualifier", "Write FROM "+ident.Table.Name)
	}
	rel := relation(strings.ToLower(ident.Table.Name))
	switch rel {
	case relationSpans, relationTraces, relationEvents, relationLinks:
	default:
		return "", "", c.pos.at(ident, fmt.Sprintf("unknown relation %q", ident.Table.Name), "The relations are spans, traces, and events and links inside EXISTS")
	}
	if alias == "" {
		alias = string(rel)
	}
	return rel, alias, nil
}

// spans compiles a statement over spans: the window, the filter, and the shape that picks
// FindTraceIDs or FindSpans.
func (c *compiler) spans(sel *parser.SelectQuery, sc *scope) (*Query, error) {
	query := &Query{}
	switch {
	case sel.HasDistinct:
		if len(sel.SelectItems) != 1 || !c.isColumnNamed(sel.SelectItems[0].Expr, "traceid") {
			return nil, c.pos.at(sel, "DISTINCT is accepted only as SELECT DISTINCT trace_id", "SELECT DISTINCT trace_id FROM spans returns trace IDs; SELECT * FROM spans returns spans")
		}
		query.Kind = KindFindTraceIDs
	case len(sel.SelectItems) == 1 && isStar(sel.SelectItems[0].Expr):
		query.Kind = KindFindSpans
	default:
		query.Kind = KindFindSpans
		for _, item := range sel.SelectItems {
			if err := c.refuseSelectItem(item); err != nil {
				return nil, err
			}
			ref, err := c.reference(sc, item.Expr)
			if err != nil {
				return nil, err
			}
			query.Projection = append(query.Projection, ref)
		}
	}
	if err := c.where(sel, sc, query); err != nil {
		return nil, err
	}
	if sel.OrderBy != nil {
		if query.Kind != KindFindSpans {
			return nil, c.pos.at(sel.OrderBy, "ORDER BY applies to a span search", "Write SELECT * FROM spans to order spans; a trace-ID search has no order")
		}
		order, err := c.order(sel.OrderBy, sc)
		if err != nil {
			return nil, err
		}
		query.OrderBy = order
	}
	limit, err := c.limit(sel.Limit)
	if err != nil {
		return nil, err
	}
	query.Limit = limit
	return query, nil
}

// refuseSelectItem rejects a select item that is not a column, naming aggregation when that
// is what the item is.
func (c *compiler) refuseSelectItem(item *parser.SelectItem) error {
	if fn, ok := item.Expr.(*parser.FunctionExpr); ok && isAggregate(strings.ToLower(fn.Name.Name)) {
		return c.pos.at(item, fn.Name.Name+"() is an aggregate", hintAggregation)
	}
	if !isReference(item.Expr) {
		return c.pos.at(item, "SELECT lists columns, or *", "Expressions in SELECT have no target in the result; the result is the RPC's")
	}
	if item.Alias != nil {
		return c.pos.at(item.Alias, "a column alias in SELECT has no effect", "The result's field names are the RPC's")
	}
	return nil
}

// where splits WHERE into the window, which the request carries beside the filter, and the
// filter. Both bounds on start_time of the statement's alias are required (RFC 0019 §4.2).
func (c *compiler) where(sel *parser.SelectQuery, sc *scope, query *Query) error {
	if sel.Where == nil {
		return c.pos.at(sel, "WHERE is required, with both bounds on start_time", hintWindow)
	}
	var predicates []*ast.Call
	for _, conjunct := range conjuncts(sel.Where.Expr) {
		isBound, err := c.windowBound(sc, conjunct, query)
		if err != nil {
			return err
		}
		if isBound {
			continue
		}
		pred, err := c.predicate(sc, conjunct)
		if err != nil {
			return err
		}
		predicates = append(predicates, pred)
	}
	switch {
	case query.StartTimeMin.IsZero() && query.StartTimeMax.IsZero():
		return c.pos.at(sel.Where, "WHERE needs both bounds on start_time", hintWindow)
	case query.StartTimeMin.IsZero():
		return c.pos.at(sel.Where, "WHERE needs a lower bound on start_time", hintWindow)
	case query.StartTimeMax.IsZero():
		return c.pos.at(sel.Where, "WHERE needs an upper bound on start_time", hintWindow)
	case !query.StartTimeMin.Before(query.StartTimeMax):
		return c.pos.at(sel.Where, "the start_time window is empty", "The lower bound is not before the upper bound")
	}
	if len(predicates) == 0 {
		return nil
	}
	filter, err := tracestore.FinalizeFilter(builder.And(predicates...))
	if err != nil {
		return c.pos.at(sel.Where, "the filter is not valid: "+err.Error(), "")
	}
	query.Filter = filter
	return nil
}

// windowBound recognizes a top-level conjunct that bounds start_time of the statement's alias
// against an instant and records it. A start_time compared with another column stays in the
// filter, and a second bound on the same side is refused.
func (c *compiler) windowBound(sc *scope, node parser.Expr, query *Query) (bool, error) {
	if between, ok := node.(*parser.BetweenClause); ok && !between.Not && c.isColumnNamed(between.Expr, "starttime") && !isReference(between.Between) && !isReference(between.And) {
		low, err := c.instantOf(between.Between)
		if err != nil {
			return false, err
		}
		high, err := c.instantOf(between.And)
		if err != nil {
			return false, err
		}
		if err := c.setBound(node, query, &query.StartTimeMin, low); err != nil {
			return false, err
		}
		return true, c.setBound(node, query, &query.StartTimeMax, high)
	}
	bin, ok := node.(*parser.BinaryOperation)
	if !ok {
		return false, nil
	}
	op := string(bin.Operation)
	ref, value := bin.LeftExpr, bin.RightExpr
	if !c.isColumnNamed(ref, "starttime") {
		ref, value, op = bin.RightExpr, bin.LeftExpr, mirror(op)
	}
	if !c.isColumnNamed(ref, "starttime") || isReference(value) || !sameScope(sc, ref) {
		return false, nil
	}
	if op != ">=" && op != ">" && op != "<" && op != "<=" {
		return false, nil
	}
	instant, err := c.instantOf(value)
	if err != nil {
		return false, err
	}
	if op == ">=" || op == ">" {
		return true, c.setBound(node, query, &query.StartTimeMin, instant)
	}
	return true, c.setBound(node, query, &query.StartTimeMax, instant)
}

func (c *compiler) setBound(node parser.Expr, _ *Query, bound *time.Time, value time.Time) error {
	if !bound.IsZero() {
		return c.pos.at(node, "start_time is bounded twice on the same side", "Write one lower bound and one upper bound")
	}
	*bound = value
	return nil
}

// instantOf evaluates the right-hand side of a window bound: an RFC 3339 string, or now()
// arithmetic.
func (c *compiler) instantOf(node parser.Expr) (time.Time, error) {
	if t, ok, err := c.instant(node); ok || err != nil {
		return t, err
	}
	lit, ok := node.(*parser.StringLiteral)
	if !ok {
		return time.Time{}, c.pos.at(node, "a start_time bound is an instant", "Write an RFC 3339 string such as '2026-10-23T12:00:00Z', or now() - '1h'")
	}
	t, err := time.Parse(time.RFC3339Nano, unquote(lit.Literal))
	if err != nil {
		return time.Time{}, c.pos.at(node, fmt.Sprintf("%q is not an RFC 3339 instant", unquote(lit.Literal)), "Write '2026-10-23T12:00:00Z', with the offset")
	}
	return t, nil
}

// isColumnNamed reports whether a node is a plain column reference with the given key,
// whatever its alias.
func (*compiler) isColumnNamed(node parser.Expr, key string) bool {
	switch ref := node.(type) {
	case *parser.Ident:
		return columnKey(ref.Name) == key
	case *parser.Path:
		return len(ref.Fields) == 2 && columnKey(ref.Fields[1].Name) == key
	}
	return false
}

// sameScope reports whether a column reference is unqualified or qualified by the scope's alias.
func sameScope(sc *scope, node parser.Expr) bool {
	path, ok := node.(*parser.Path)
	return !ok || strings.EqualFold(path.Fields[0].Name, sc.alias)
}

func isStar(node parser.Expr) bool {
	ident, ok := node.(*parser.Ident)
	return ok && ident.Name == "*"
}

// order lowers ORDER BY to the span order of RFC 0016 §6.
func (c *compiler) order(clause *parser.OrderByClause, sc *scope) ([]tracestore.SpanSortOrder, error) {
	var order []tracestore.SpanSortOrder
	for _, item := range clause.Items {
		term, ok := item.(*parser.OrderExpr)
		if !ok {
			return nil, c.pos.at(item, "ORDER BY takes columns", "")
		}
		if term.Fill != nil {
			return nil, c.pos.at(term.Expr, "WITH FILL is not supported", "")
		}
		name := columnName(term.Expr)
		if name == "" || !isSortableColumn(name) {
			return nil, c.pos.at(term.Expr, "ORDER BY takes a sortable column", "The sortable columns are start_time, duration, trace_id and span_id (RFC 0016 §6)")
		}
		ref, err := c.reference(sc, term.Expr)
		if err != nil {
			return nil, err
		}
		direction := tracestore.SortAscending
		if term.Direction == parser.OrderDirectionDesc {
			direction = tracestore.SortDescending
		}
		order = append(order, tracestore.SpanSortOrder{Expression: ref, Direction: direction})
	}
	normalized, err := tracestore.NormalizeSpanOrder(order)
	if err != nil {
		return nil, c.pos.at(clause, "the order is not valid: "+err.Error(), "")
	}
	return normalized, nil
}

func columnName(node parser.Expr) string {
	switch ref := node.(type) {
	case *parser.Ident:
		return ref.Name
	case *parser.Path:
		if len(ref.Fields) == 2 {
			return ref.Fields[1].Name
		}
		return ""
	default:
		return ""
	}
}

// limit reads LIMIT n. OFFSET was refused with the other clauses.
func (c *compiler) limit(clause *parser.LimitClause) (uint32, error) {
	if clause == nil {
		return 0, nil
	}
	lit, ok := clause.Limit.(*parser.NumberLiteral)
	if !ok {
		return 0, c.pos.at(clause.Limit, "LIMIT takes a whole number", "")
	}
	n, err := strconv.ParseUint(lit.Literal, 10, 32)
	if err != nil || n == 0 {
		return 0, c.pos.at(clause.Limit, fmt.Sprintf("%q is not a page size", lit.Literal), "LIMIT takes a whole number from 1 up; the server caps it at its configured maximum")
	}
	return uint32(n), nil
}

// traces compiles the one shape over traces: SELECT ... FROM traces WHERE trace_id IN (SELECT
// trace_id FROM spans WHERE ...), which is FindTraces for * and FindTraceSummaries for a column
// list (RFC 0019 §4.4).
func (c *compiler) traces(sel *parser.SelectQuery, alias string) (*Query, error) {
	const shape = "SELECT * FROM traces WHERE trace_id IN (SELECT trace_id FROM spans WHERE ...)"
	query := &Query{Kind: KindFindTraces}
	if len(sel.SelectItems) != 1 || !isStar(sel.SelectItems[0].Expr) {
		query.Kind = KindFindTraceSummaries
		for _, item := range sel.SelectItems {
			if err := c.refuseSelectItem(item); err != nil {
				return nil, err
			}
			name := columnName(item.Expr)
			if name == "" || !isTracesColumn(name) {
				return nil, c.pos.at(item, "traces has no column "+quoteName(item.Expr), "Its columns are "+columnNames(relationTraces))
			}
		}
	}
	if sel.OrderBy != nil {
		return nil, c.pos.at(sel.OrderBy, "a trace search has no ORDER BY", "Write SELECT * FROM spans to order spans")
	}
	if sel.Where == nil {
		return nil, c.pos.at(sel, "a traces query is "+shape, "A trace predicate alone has nothing to search on; the subquery over spans does the searching")
	}
	bin, ok := sel.Where.Expr.(*parser.BinaryOperation)
	if !ok || !strings.EqualFold(string(bin.Operation), parser.KeywordIn) || !c.isColumnNamed(bin.LeftExpr, "traceid") || !sameScope(&scope{alias: alias}, bin.LeftExpr) {
		return nil, c.pos.at(sel.Where, "the WHERE of a traces query is trace_id IN (SELECT trace_id FROM spans WHERE ...)", "Put every predicate inside the subquery; it is what selects the traces")
	}
	sub, ok := bin.RightExpr.(*parser.SubQuery)
	if !ok {
		return nil, c.pos.at(bin.RightExpr, "trace_id IN takes the subquery (SELECT trace_id FROM spans WHERE ...)", "")
	}
	if err := c.refuseClauses(sub.Select); err != nil {
		return nil, err
	}
	rel, subAlias, err := c.relationOf(sub.Select)
	if err != nil {
		return nil, err
	}
	if rel != relationSpans {
		return nil, c.pos.at(sub.Select.From, "the subquery of a traces query reads spans", shape)
	}
	if len(sub.Select.SelectItems) != 1 || !c.isColumnNamed(sub.Select.SelectItems[0].Expr, "traceid") {
		return nil, c.pos.at(sub.Select, "the subquery of a traces query selects trace_id", shape)
	}
	if sub.Select.OrderBy != nil || sub.Select.Limit != nil {
		return nil, c.pos.at(sub.Select, "ORDER BY and LIMIT go on the outer traces query", "LIMIT there is the search depth, the number of traces returned")
	}
	if err := c.where(sub.Select, &scope{alias: subAlias, rel: relationSpans}, query); err != nil {
		return nil, err
	}
	limit, err := c.limit(sel.Limit)
	if err != nil {
		return nil, err
	}
	query.Limit = limit
	return query, nil
}

func quoteName(node parser.Expr) string {
	return strconv.Quote(formatted(node))
}

// formatted prints one node back as SQL.
func formatted(node parser.Expr) string {
	f := parser.NewFormatter()
	node.FormatSQL(f)
	return f.String()
}
