// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// filterCtx carries what a filter predicate is evaluated against: a span in
// the context of its resource and instrumentation scope, plus, inside a
// `some` quantifier, the single event or link the quantifier currently binds
// (RFC 0005 §5.5). Outside `some`, boundEvent and boundLink are nil and an
// event/link-level reference resolves against every event or link on the
// span (the "uncorrelated, any element" meaning §5.5 documents).
//
// resourceSchemaURL and scopeSchemaURL come from the enclosing ResourceSpans
// and ScopeSpans, not from pcommon.Resource/InstrumentationScope themselves,
// which carry no schema URL of their own (it lives on the wrapper OTLP carries
// it in). A caller that declares resource.schemaURL/scope.schemaURL supported
// has to supply them here for that declaration to be true.
type filterCtx struct {
	resource          pcommon.Resource
	scope             pcommon.InstrumentationScope
	span              ptrace.Span
	resourceSchemaURL string
	scopeSchemaURL    string

	boundEvent *ptrace.SpanEvent
	boundLink  *ptrace.SpanLink

	regexps map[*expression.Call]*regexp.Regexp
}

// evalKind is the kind of value an evalValue holds. The order of the kinds is part of the
// contract: a sorting key encodes the kind as its first byte, so spans without a value come
// first, then booleans, integers, doubles and strings.
type evalKind uint8

const (
	// kindNone is the zero value and means no value, as when an operand resolves to nothing.
	kindNone evalKind = iota
	kindBool
	kindInt
	kindDouble
	kindString
	// kindOpaque marks a present attribute (bytes, slice or map — the OTLP types a filter
	// predicate has no scalar reading for) that resolveAttributeRef still has to report as
	// present, since OpExists checks presence rather than comparability. It is never comparable:
	// resolveComparable refuses any pair that includes one.
	kindOpaque
	// kindUntyped marks an AnyValue constant, which the caller wrote under no type constraint
	// (RFC 0005 §5.4). Its str field holds the raw text as written; resolveComparable resolves
	// that text against the paired operand's actual kind before a comparison runs, rather than
	// treating it as a string, which is what made an untyped `"500"` fail to match a numeric or
	// boolean attribute that happened to hold that value.
	kindUntyped
)

// evalValue is a resolved operand: a constant from the filter or a value read off the span. The
// kind says which payload field holds the value. A kindUntyped value is resolved into one of the
// other kinds before compareValues or valueInList ever sees it — see resolveComparable and
// coerceUntyped.
//
// kindInt and kindDouble are kept apart, rather than folded into one float64 field, because an
// integer read as float64 loses precision once it passes 2^53: two adjacent current-era
// nanosecond timestamps, a millisecond apart in wall-clock terms but one nanosecond apart in
// their stored int64, would otherwise compare equal. kindInt carries every exact integer — an
// IntValue constant, a Duration or Timestamp constant resolved to nanoseconds, an int-valued
// attribute, and every span/event time or duration field — at full int64 precision. kindDouble
// is only ever a DoubleValue constant or a double-valued attribute, which are floating-point by
// nature and have no exact form to preserve.
type evalValue struct {
	kind evalKind
	// typed marks an IntValue or DoubleValue constant. Its declared type is authoritative
	// (RFC 0005 §5.4), so it matches only an attribute stored as that numeric type, while two
	// attributes of different numeric types still compare with each other.
	typed   bool
	str     string
	numInt  int64
	num     float64
	boolean bool
}

func stringValue(s string) evalValue  { return evalValue{kind: kindString, str: s} }
func intValue(n int64) evalValue      { return evalValue{kind: kindInt, numInt: n} }
func doubleValue(f float64) evalValue { return evalValue{kind: kindDouble, num: f} }
func boolValue(b bool) evalValue      { return evalValue{kind: kindBool, boolean: b} }
func untypedValue(s string) evalValue { return evalValue{kind: kindUntyped, str: s} }

// typed marks a numeric constant whose type the filter declared.
func typed(v evalValue) evalValue {
	v.typed = true
	return v
}

// isNumeric reports whether v is an integer or a double. The two compare with each other, since
// an int and a double are still the same broad kind of thing to compare, unlike a string or a
// bool.
func (v evalValue) isNumeric() bool {
	return v.kind == kindInt || v.kind == kindDouble
}

// comparableKinds reports whether two resolved values can be compared at all: both numeric, or
// both strings, or both booleans. An opaque or still-untyped value compares with nothing.
func comparableKinds(a, b evalValue) bool {
	if a.isNumeric() && b.isNumeric() {
		return true
	}
	return a.kind == b.kind && (a.kind == kindString || a.kind == kindBool)
}

// validateFilterShape checks a filter's shape once, before any span is evaluated against it: an
// unsupported operator, an operator given the wrong number of arguments, or a boolean combinator
// (and/or/not) or some's quantified predicate given something other than a nested predicate.
// Nothing dynamic is checked here — a value's actual type, or whether a reference resolves to
// anything, stays evalPredicate's per-span job (RFC 0005 §5.3 makes those questions the data's to
// answer, not the filter's own). This is a static property of the filter alone, the same
// separation the elasticsearch backend's buildFilterQuery draws by lowering to its query once
// rather than per span.
//
// The query service already validates a filter's shape (RFC 0005 §7) before a Reader ever sees
// it, but a remote-storage client can reach this reader without that check, so this refuses the
// same shapes again rather than assuming they were already ruled out — the same reasoning
// buildFilterQuery's own doc comment gives for repeating its refusals.
func validateFilterShape(filter *expression.Call) error {
	if filter == nil {
		return nil
	}
	switch filter.Op {
	case expression.OpAnd, expression.OpOr:
		if len(filter.Args) == 0 {
			return errArity(filter)
		}
		for _, arg := range filter.Args {
			call, ok := arg.(*expression.Call)
			if !ok {
				return fmt.Errorf("%w: %q combines predicates, not values", tracestore.ErrFilterInvalid, filter.Op)
			}
			if err := validateFilterShape(call); err != nil {
				return err
			}
		}
	case expression.OpNot:
		if len(filter.Args) != 1 {
			return errArity(filter)
		}
		call, ok := filter.Args[0].(*expression.Call)
		if !ok {
			return fmt.Errorf("%w: %q negates a predicate, not a value", tracestore.ErrFilterInvalid, filter.Op)
		}
		return validateFilterShape(call)
	case expression.OpExists:
		if len(filter.Args) != 1 {
			return errArity(filter)
		}
	case expression.OpEq, expression.OpNe, expression.OpGt, expression.OpLt,
		expression.OpGte, expression.OpLte, expression.OpRegex, expression.OpIn, expression.OpNotIn:
		if len(filter.Args) != 2 {
			return errArity(filter)
		}
	case expression.OpSome:
		if len(filter.Args) != 2 {
			return errArity(filter)
		}
		if _, ok := filter.Args[0].(*expression.NestedRef); !ok {
			return fmt.Errorf("%w: %q quantifies over an event or link collection",
				tracestore.ErrFilterInvalid, filter.Op)
		}
		call, ok := filter.Args[1].(*expression.Call)
		if !ok {
			return fmt.Errorf("%w: %q quantifies a predicate, not a value", tracestore.ErrFilterInvalid, filter.Op)
		}
		return validateFilterShape(call)
	default:
		return fmt.Errorf("%w: it does not support the operator %q", tracestore.ErrFilterUnsupported, filter.Op)
	}
	return nil
}

func errArity(filter *expression.Call) error {
	return fmt.Errorf("%w: %q cannot take %d arguments",
		tracestore.ErrFilterInvalid, filter.Op, len(filter.Args))
}

// preparedFilter is a filter whose shape has been validated and whose regular
// expressions have been compiled, once per search rather than once per span.
type preparedFilter struct {
	call    *expression.Call
	regexps map[*expression.Call]*regexp.Regexp
}

// prepareFilter validates the filter's shape and compiles the pattern of every
// regex predicate. A nil filter prepares to one that matches every span.
func prepareFilter(filter *expression.Call) (preparedFilter, error) {
	if err := validateFilterShape(filter); err != nil {
		return preparedFilter{}, err
	}
	if filter != nil {
		resolved, err := tracestore.ResolveFilterConstants(filter)
		if err != nil {
			return preparedFilter{}, fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
		}
		filter = resolved
	}
	p := preparedFilter{call: filter, regexps: map[*expression.Call]*regexp.Regexp{}}
	if err := p.compileRegexps(filter); err != nil {
		return preparedFilter{}, err
	}
	return p, nil
}

func (p preparedFilter) compileRegexps(filter *expression.Call) error {
	if filter == nil {
		return nil
	}
	if filter.Op == expression.OpRegex {
		var pattern string
		switch v := filter.Args[1].(type) {
		case *expression.StringValue:
			pattern = v.Value
		case *expression.AnyValue:
			pattern = v.Value
		default:
			return fmt.Errorf("%w: %q takes a constant pattern", tracestore.ErrFilterInvalid, filter.Op)
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("%w: %q takes a pattern in RE2 syntax: %w", tracestore.ErrFilterInvalid, filter.Op, err)
		}
		p.regexps[filter] = re
		return nil
	}
	for _, arg := range filter.Args {
		if call, ok := arg.(*expression.Call); ok {
			if err := p.compileRegexps(call); err != nil {
				return err
			}
		}
	}
	return nil
}

// matchesFilter reports whether a span, in the context of its resource and
// scope, satisfies filter.
func matchesFilter(
	filter preparedFilter,
	resource pcommon.Resource,
	scope pcommon.InstrumentationScope,
	span ptrace.Span,
	resourceSchemaURL, scopeSchemaURL string,
) bool {
	if filter.call == nil {
		return true
	}
	ctx := filterCtx{
		resource: resource, scope: scope, span: span,
		resourceSchemaURL: resourceSchemaURL, scopeSchemaURL: scopeSchemaURL,
		regexps: filter.regexps,
	}
	return evalPredicate(filter.call, ctx)
}

// evalPredicate evaluates a boolean-valued expression: a Call applying one of
// the operators in expression.Operators(). Only Call reaches here; a bare
// reference or constant is never itself a predicate (the query boundary
// requires every predicate to be a Call).
func evalPredicate(expr expression.Expression, ctx filterCtx) bool {
	call, ok := expr.(*expression.Call)
	if !ok {
		return false
	}
	switch call.Op {
	case expression.OpAnd:
		for _, arg := range call.Args {
			if !evalPredicate(arg, ctx) {
				return false
			}
		}
		return true
	case expression.OpOr:
		for _, arg := range call.Args {
			if evalPredicate(arg, ctx) {
				return true
			}
		}
		return false
	case expression.OpNot:
		return !evalPredicate(call.Args[0], ctx)
	case expression.OpExists:
		return len(resolveOperand(call.Args[0], ctx)) > 0
	case expression.OpEq:
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c == 0 })
	case expression.OpNe:
		return leafPresentAndNoPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c == 0 })
	case expression.OpGt:
		return orderedPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c > 0 })
	case expression.OpLt:
		return orderedPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c < 0 })
	case expression.OpGte:
		return orderedPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c >= 0 })
	case expression.OpLte:
		return orderedPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c <= 0 })
	case expression.OpRegex:
		return evalRegex(call, ctx)
	case expression.OpIn:
		return evalIn(call.Args[0], call.Args[1], ctx)
	case expression.OpNotIn:
		return evalNotIn(call.Args[0], call.Args[1], ctx)
	case expression.OpSome:
		return evalSome(call.Args[0], call.Args[1], ctx)
	default:
		// Unreached for a filter the query boundary admitted: it only ever names
		// one of the operators above. Refusing to match, rather than panicking,
		// keeps an operator this evaluator has not learned yet from taking down
		// a search instead of just under-matching it.
		return false
	}
}

// anyPairMatches implements the positive-leaf rule (RFC 0005 §5.3): the
// predicate holds when some combination of the left and right operands'
// resolved values satisfies test, once resolveComparable has paired them.
// Either side resolving to nothing (an absent reference), or every pair
// being incomparable (an opaque attribute, or a type mismatch), makes the
// predicate false, since a leaf comparison other than a boolean `not` never
// turns absence or incomparability into a match.
func anyPairMatches(left, right expression.Expression, ctx filterCtx, test func(c int) bool) bool {
	return anyComparablePair(left, right, ctx, func(a, b evalValue) bool { return test(compareValues(a, b)) })
}

// orderedPairMatches is anyPairMatches for gt, lt, gte and lte. A boolean has no
// order (RFC 0005 §5.3); the query boundary refuses ordering a boolean built-in
// field, but an attribute's type is known only here, so a boolean pair is
// refused here rather than ordered.
func orderedPairMatches(left, right expression.Expression, ctx filterCtx, test func(c int) bool) bool {
	return anyComparablePair(left, right, ctx, func(a, b evalValue) bool { return a.kind != kindBool && test(compareValues(a, b)) })
}

func anyComparablePair(left, right expression.Expression, ctx filterCtx, test func(a, b evalValue) bool) bool {
	lv := resolveOperand(left, ctx)
	rv := resolveOperand(right, ctx)
	for _, a := range lv {
		for _, b := range rv {
			ca, cb, ok := resolveComparable(a, b)
			if ok && test(ca, cb) {
				return true
			}
		}
	}
	return false
}

// leafPresentAndNoPairMatches implements the negated-leaf rule (RFC 0005
// §5.3) that `ne` and `not_in` share: the predicate holds only when the
// reference side is present (resolves to at least one value, comparable or
// not) and none of its values match. This is deliberately not
// "not(anyPairMatches(...))" — an absent reference must evaluate to false,
// the same as a positive leaf, and only a boolean `not` around the whole
// comparison flips that. Presence is checked before resolveComparable runs,
// so a present-but-opaque attribute (bytes, slice, map) still counts as
// present: it is definitely not equal to anything scalar, which is a `ne`
// match, not an absence.
func leafPresentAndNoPairMatches(left, right expression.Expression, ctx filterCtx, test func(c int) bool) bool {
	lv := resolveOperand(left, ctx)
	rv := resolveOperand(right, ctx)
	if len(lv) == 0 || len(rv) == 0 {
		return false
	}
	for _, a := range lv {
		for _, b := range rv {
			ca, cb, ok := resolveComparable(a, b)
			if ok && test(compareValues(ca, cb)) {
				return false
			}
		}
	}
	return true
}

// resolveComparable prepares a pair of resolved operands for compareValues.
// It resolves either side's untyped value (an AnyValue) against the
// other's actual kind, and reports ok=false when the pair cannot be
// meaningfully compared at all: one side is opaque, the untyped text does
// not parse as the other side's kind, or, once resolved, the two sides are
// still different kinds.
//
// RFC 0005's "both operands hold the same kind" rule is enforced at the
// query boundary, but only for two built-in fields, whose types are known
// statically. An attribute's actual type is stored data, resolved only here,
// at match time — the query boundary "intentionally leaves attribute types
// for storage to resolve" — so a comparison against one cannot rely on that
// check having already run, and this function is what actually enforces it
// for that case.
func resolveComparable(a, b evalValue) (resolvedA, resolvedB evalValue, ok bool) {
	if a.kind == kindOpaque || b.kind == kindOpaque {
		return evalValue{}, evalValue{}, false
	}
	if a.kind == kindUntyped {
		resolved, ok := coerceUntyped(a, b)
		if !ok {
			return evalValue{}, evalValue{}, false
		}
		a = resolved
	}
	if b.kind == kindUntyped {
		resolved, ok := coerceUntyped(b, a)
		if !ok {
			return evalValue{}, evalValue{}, false
		}
		b = resolved
	}
	if !comparableKinds(a, b) {
		return evalValue{}, evalValue{}, false
	}
	if (a.typed || b.typed) && a.kind != b.kind {
		return evalValue{}, evalValue{}, false
	}
	// NaN is neither below, above nor equal to anything, so it cannot be compared at all.
	if (a.kind == kindDouble && math.IsNaN(a.num)) || (b.kind == kindDouble && math.IsNaN(b.num)) {
		return evalValue{}, evalValue{}, false
	}
	return a, b, true
}

// coerceUntyped resolves v, an AnyValue's raw text, against other's kind: a
// numeric other reads v.str as a number, a boolean other reads it as a bool,
// and a string other (or one that is itself still untyped, such as a second
// AnyValue) takes v.str as-is. It reports ok=false only when other calls for
// a numeric or boolean reading and v.str cannot be read that way.
func coerceUntyped(v, other evalValue) (evalValue, bool) {
	switch {
	case other.kind == kindBool:
		b, err := strconv.ParseBool(v.str)
		if err != nil {
			return evalValue{}, false
		}
		return boolValue(b), true
	case other.isNumeric():
		// The text takes the other side's numeric type, so that beside a double it is a
		// double even when it is written without a fraction.
		if other.kind == kindInt {
			if n, err := strconv.ParseInt(v.str, 10, 64); err == nil {
				return intValue(n), true
			}
		}
		f, err := strconv.ParseFloat(v.str, 64)
		if err != nil {
			return evalValue{}, false
		}
		return doubleValue(f), true
	default:
		// A string, or an operand that is itself still untyped (a second AnyValue), in which
		// case it resolves against v's own text next and the pair ends up compared as raw
		// strings. An opaque other is excluded by resolveComparable before this is called.
		return stringValue(v.str), true
	}
}

// isIntegral reports whether v is a double that an int64 holds exactly.
func isIntegral(v evalValue) bool {
	return v.kind == kindDouble && v.num == math.Trunc(v.num) && v.num >= -(1<<63) && v.num < 1<<63
}

func evalRegex(call *expression.Call, ctx filterCtx) bool {
	values := resolveOperand(call.Args[0], ctx)
	if len(values) == 0 {
		return false
	}
	re := ctx.regexps[call]
	for _, v := range values {
		if v.kind == kindString && re.MatchString(v.str) {
			return true
		}
	}
	return false
}

func evalIn(ref, listExpr expression.Expression, ctx filterCtx) bool {
	values, list, ok := resolveMembership(ref, listExpr, ctx)
	if !ok {
		return false
	}
	for _, v := range values {
		if inList(ref, v, list) {
			return true
		}
	}
	return false
}

// evalNotIn is the negated leaf beside evalIn, as leafPresentAndNoPairMatches is beside
// anyPairMatches: it holds only when the reference is present and none of its values is in
// the list (RFC 0005 §5.3).
func evalNotIn(ref, listExpr expression.Expression, ctx filterCtx) bool {
	values, list, ok := resolveMembership(ref, listExpr, ctx)
	if !ok {
		return false
	}
	for _, v := range values {
		if inList(ref, v, list) {
			return false
		}
	}
	return true
}

// resolveMembership resolves the operands of in and not_in, reporting false when the
// reference is absent or the right operand is not a list.
func resolveMembership(ref, listExpr expression.Expression, ctx filterCtx) ([]evalValue, *expression.List, bool) {
	values := resolveOperand(ref, ctx)
	if len(values) == 0 {
		return nil, nil, false
	}
	list, ok := listExpr.(*expression.List)
	if !ok {
		return nil, nil, false
	}
	return values, list, true
}

// inList reports whether v matches one of list's elements. Beside a duration or
// timestamp field the elements are read as that field's type, since a list carries
// its elements as text and has no type of its own for them (tracestore.ReadFilterElement);
// everywhere else valueInList applies.
func inList(ref expression.Expression, v evalValue, list *expression.List) bool {
	fieldType, ok := timeFieldType(ref)
	if !ok {
		return valueInList(v, list)
	}
	for _, elem := range list.Values {
		typed, err := tracestore.ReadFilterElement(list, fieldType, elem)
		if err != nil {
			continue
		}
		for _, e := range resolveOperand(typed, filterCtx{}) {
			if a, b, ok := resolveComparable(v, e); ok && compareValues(a, b) == 0 {
				return true
			}
		}
	}
	return false
}

// timeFieldType returns the type of the built-in field ref names when that is a
// duration or a timestamp, the two types whose constants travel as text.
func timeFieldType(ref expression.Expression) (expression.FieldType, bool) {
	r, ok := ref.(*expression.FieldRef)
	if !ok || r == nil {
		return "", false
	}
	// An unknown field has no type and falls through; it also resolves to no value, so
	// the caller never reaches the list.
	field, _ := expression.LookupField(r.Level, r.Name)
	if field.Type == expression.FieldTypeDuration || field.Type == expression.FieldTypeTimestamp {
		return field.Type, true
	}
	return "", false
}

// valueInList reports whether v matches one of list's elements. list.Type,
// when set, is authoritative (RFC 0005 §5.4): only a value of that declared
// kind may match, and each element is parsed as that kind — a numeric
// attribute does not match a string-typed list containing its digits, and a
// string attribute does not match an int-typed list the same way. An empty
// Type means each element is read against v the way an untyped scalar beside
// an attribute is (resolveComparable): neither the list nor an attribute
// reference has a static type to supply one otherwise.
func valueInList(v evalValue, list *expression.List) bool {
	if list.Type == "" {
		// An untyped list is a disjunction of untyped equalities (RFC 0005 §5.4), so each
		// element is read against v exactly as an untyped scalar beside v is, and `in`
		// matches whatever `eq` would.
		for _, elem := range list.Values {
			a, b, ok := resolveComparable(v, untypedValue(elem))
			if ok && compareValues(a, b) == 0 {
				return true
			}
		}
		return false
	}
	switch list.Type {
	case expression.ValueTypeString:
		return v.kind == kindString && slices.Contains(list.Values, v.str)
	case expression.ValueTypeInt:
		if v.kind != kindInt {
			return false
		}
		for _, elem := range list.Values {
			if n, err := strconv.ParseInt(elem, 10, 64); err == nil && v.numInt == n {
				return true
			}
		}
		return false
	case expression.ValueTypeDouble:
		if v.kind != kindDouble {
			return false
		}
		for _, elem := range list.Values {
			if n, err := strconv.ParseFloat(elem, 64); err == nil && v.num == n {
				return true
			}
		}
		return false
	case expression.ValueTypeBool:
		if v.kind != kindBool {
			return false
		}
		for _, elem := range list.Values {
			if b, err := strconv.ParseBool(elem); err == nil && v.boolean == b {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// evalSome quantifies pred over every element of the event or link collection
// collectionExpr names, binding the quantified level so a reference inside
// pred at that level reads the current element rather than every element on
// the span (RFC 0005 §5.5). It returns true as soon as one element satisfies
// pred.
func evalSome(collectionExpr, pred expression.Expression, ctx filterCtx) bool {
	nested, ok := collectionExpr.(*expression.NestedRef)
	if !ok {
		return false
	}
	switch nested.Level {
	case expression.LevelEvent:
		for i := 0; i < ctx.span.Events().Len(); i++ {
			event := ctx.span.Events().At(i)
			bound := ctx
			bound.boundEvent = &event
			if evalPredicate(pred, bound) {
				return true
			}
		}
	case expression.LevelLink:
		for i := 0; i < ctx.span.Links().Len(); i++ {
			link := ctx.span.Links().At(i)
			bound := ctx
			bound.boundLink = &link
			if evalPredicate(pred, bound) {
				return true
			}
		}
	default:
		// some's first operand is validated to be an event- or link-level
		// NestedRef before a filter reaches storage (RFC 0005 §5.5); any other
		// level is unreached in practice.
		return false
	}
	return false
}

// resolveOperand resolves expr to every value it denotes in ctx. A constant
// always resolves to exactly one value. A reference may resolve to none (the
// value is absent), one, or several: an unqualified attribute reference
// checks both the span and resource maps, and an event/link-level reference
// outside a `some` binding checks every event or link on the span (RFC 0005
// §5.1, §5.5).
func resolveOperand(expr expression.Expression, ctx filterCtx) []evalValue {
	switch e := expr.(type) {
	case *expression.StringValue:
		return []evalValue{stringValue(e.Value)}
	case *expression.IntValue:
		return []evalValue{typed(intValue(e.Value))}
	case *expression.DoubleValue:
		return []evalValue{typed(doubleValue(e.Value))}
	case *expression.BoolValue:
		return []evalValue{boolValue(e.Value)}
	case *expression.DurationValue:
		return []evalValue{intValue(e.Value.Nanoseconds())}
	case *expression.TimestampValue:
		return []evalValue{intValue(e.Value.UnixNano())}
	case *expression.AnyValue:
		return []evalValue{untypedValue(e.Value)}
	case *expression.FieldRef:
		return resolveFieldRef(*e, ctx)
	case *expression.AttributeRef:
		return resolveAttributeRef(*e, ctx)
	default:
		// NestedRef only ever appears as some's first operand, handled in
		// evalSome directly rather than through resolveOperand.
		return nil
	}
}

// attrToEvalValue reads an attribute's stored value at full precision: an int
// attribute keeps its exact int64 rather than passing through float64, where
// it could lose precision above 2^53. Bytes, slice and map attributes have no
// scalar reading a filter predicate can compare against, but the attribute is
// still present, so they resolve to an opaque value rather than being
// dropped — OpExists has to see them, and OpNe has to treat them as "present
// and unequal" rather than absent.
func attrToEvalValue(v pcommon.Value) evalValue {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return stringValue(v.Str())
	case pcommon.ValueTypeInt:
		return intValue(v.Int())
	case pcommon.ValueTypeDouble:
		return doubleValue(v.Double())
	case pcommon.ValueTypeBool:
		return boolValue(v.Bool())
	default:
		return evalValue{kind: kindOpaque}
	}
}

// errorVirtualAttribute reports the "error" tag's value at the span or unqualified level, the one
// attribute this store never reads out of a span's own attribute map. It is derived from the
// span's status instead (RFC 0005 does not define a built-in status-derived field, but every
// other backend accepts "error" as if it were an attribute, so this store does too, the same way
// the legacy Attributes-based search and the elasticsearch backend's asErrorTagEquality both do):
// error=true is exactly Status().Code() == Error, and error=false is every other status,
// including Unset, which is by far the common case and would otherwise be excluded. Resolving
// through the general evalValue machinery, rather than special-casing just OpEq the way the
// elasticsearch backend does, means exists (always true), ne, in and not_in all get the right
// answer for free rather than each needing their own case.
func errorVirtualAttribute(span ptrace.Span) []evalValue {
	return []evalValue{boolValue(span.Status().Code() == ptrace.StatusCodeError)}
}

func resolveAttributeRef(ref expression.AttributeRef, ctx filterCtx) []evalValue {
	if ref.Level == "" || ref.Level == expression.LevelSpan {
		// The legacy predicate fields spell span kind, span status and the scope's name and
		// version as attributes, and ToFilterShape carries them into a filter as attribute
		// references unchanged, so the evaluator reads them off the span the way validSpan does.
		switch ref.Key {
		case errorAttribute:
			return errorVirtualAttribute(ctx.span)
		case "span.kind":
			return resolveFieldRef(expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldKind}, ctx)
		case "span.status":
			return resolveFieldRef(expression.FieldRef{Level: expression.LevelSpan, Name: expression.SpanFieldStatus}, ctx)
		case "scope.name":
			return resolveFieldRef(expression.FieldRef{Level: expression.LevelScope, Name: expression.ScopeFieldName}, ctx)
		case "scope.version":
			return resolveFieldRef(expression.FieldRef{Level: expression.LevelScope, Name: expression.ScopeFieldVersion}, ctx)
		}
	}
	key := ref.Key
	var maps []pcommon.Map
	switch ref.Level {
	case expression.LevelSpan:
		maps = []pcommon.Map{ctx.span.Attributes()}
	case expression.LevelResource:
		maps = []pcommon.Map{ctx.resource.Attributes()}
	case expression.LevelScope:
		maps = []pcommon.Map{ctx.scope.Attributes()}
	case expression.LevelEvent:
		if ctx.boundEvent != nil {
			maps = []pcommon.Map{ctx.boundEvent.Attributes()}
		} else {
			for i := 0; i < ctx.span.Events().Len(); i++ {
				maps = append(maps, ctx.span.Events().At(i).Attributes())
			}
		}
	case expression.LevelLink:
		if ctx.boundLink != nil {
			maps = []pcommon.Map{ctx.boundLink.Attributes()}
		} else {
			for i := 0; i < ctx.span.Links().Len(); i++ {
				maps = append(maps, ctx.span.Links().At(i).Attributes())
			}
		}
	default:
		// Empty level: the unqualified span-or-resource search (RFC 0005 §5.1). The legacy
		// predicate fields spell a resource attribute with a "resource." prefix, which
		// ToFilterShape keeps, so the prefix reads the resource's attribute as validSpan does.
		if resourceKey, ok := strings.CutPrefix(key, "resource."); ok {
			key = resourceKey
			maps = []pcommon.Map{ctx.resource.Attributes()}
		} else {
			maps = []pcommon.Map{ctx.span.Attributes(), ctx.resource.Attributes()}
		}
	}
	var values []evalValue
	for _, m := range maps {
		if raw, ok := m.Get(key); ok {
			values = append(values, attrToEvalValue(raw))
		}
	}
	return values
}

func resolveFieldRef(ref expression.FieldRef, ctx filterCtx) []evalValue {
	switch ref.Level {
	case expression.LevelSpan:
		return resolveSpanField(ref.Name, ctx.span)
	case expression.LevelResource:
		return resolveResourceField(ref.Name, ctx.resource, ctx.resourceSchemaURL)
	case expression.LevelScope:
		return resolveScopeField(ref.Name, ctx.scope, ctx.scopeSchemaURL)
	case expression.LevelEvent:
		if ctx.boundEvent != nil {
			return resolveEventField(ref.Name, *ctx.boundEvent, ctx.span)
		}
		var values []evalValue
		for i := 0; i < ctx.span.Events().Len(); i++ {
			values = append(values, resolveEventField(ref.Name, ctx.span.Events().At(i), ctx.span)...)
		}
		return values
	case expression.LevelLink:
		if ctx.boundLink != nil {
			return resolveLinkField(ref.Name, *ctx.boundLink)
		}
		var values []evalValue
		for i := 0; i < ctx.span.Links().Len(); i++ {
			values = append(values, resolveLinkField(ref.Name, ctx.span.Links().At(i))...)
		}
		return values
	default:
		return nil
	}
}

func resolveSpanField(name string, span ptrace.Span) []evalValue {
	switch name {
	case expression.SpanFieldTraceID:
		return []evalValue{stringValue(span.TraceID().String())}
	case expression.SpanFieldSpanID:
		return []evalValue{stringValue(span.SpanID().String())}
	case expression.SpanFieldParentSpanID:
		if span.ParentSpanID().IsEmpty() {
			return nil
		}
		return []evalValue{stringValue(span.ParentSpanID().String())}
	case expression.SpanFieldTraceState:
		state := span.TraceState().AsRaw()
		if state == "" {
			return nil
		}
		return []evalValue{stringValue(state)}
	case expression.SpanFieldName:
		return []evalValue{stringValue(span.Name())}
	case expression.SpanFieldKind:
		// fromOTELSpanKind returns "" for an unspecified kind, which the legacy search
		// uses to mean "no kind"; the filter vocabulary names it (expression.SpanKinds).
		kind := fromOTELSpanKind(span.Kind())
		if kind == "" {
			kind = expression.SpanKinds()[0]
		}
		return []evalValue{stringValue(kind)}
	case expression.SpanFieldStartTime:
		return []evalValue{intValue(int64(span.StartTimestamp()))} //nolint:gosec // G115
	case expression.SpanFieldEndTime:
		return []evalValue{intValue(int64(span.EndTimestamp()))} //nolint:gosec // G115
	case expression.SpanFieldDuration:
		dur := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
		return []evalValue{intValue(dur.Nanoseconds())}
	case expression.SpanFieldStatus:
		return []evalValue{stringValue(statusToWord(span.Status().Code()))}
	case expression.SpanFieldStatusMessage:
		if span.Status().Message() == "" {
			return nil
		}
		return []evalValue{stringValue(span.Status().Message())}
	default:
		return nil
	}
}

func resolveResourceField(name string, resource pcommon.Resource, schemaURL string) []evalValue {
	switch name {
	case expression.ResourceFieldService:
		svc := getServiceNameFromResource(resource)
		if svc == "" {
			return nil
		}
		return []evalValue{stringValue(svc)}
	case expression.ResourceFieldSchemaURL:
		if schemaURL == "" {
			return nil
		}
		return []evalValue{stringValue(schemaURL)}
	default:
		return nil
	}
}

func resolveScopeField(name string, scope pcommon.InstrumentationScope, schemaURL string) []evalValue {
	switch name {
	case expression.ScopeFieldName:
		if scope.Name() == "" {
			return nil
		}
		return []evalValue{stringValue(scope.Name())}
	case expression.ScopeFieldVersion:
		if scope.Version() == "" {
			return nil
		}
		return []evalValue{stringValue(scope.Version())}
	case expression.ScopeFieldSchemaURL:
		if schemaURL == "" {
			return nil
		}
		return []evalValue{stringValue(schemaURL)}
	default:
		return nil
	}
}

func resolveEventField(name string, event ptrace.SpanEvent, span ptrace.Span) []evalValue {
	switch name {
	case expression.EventFieldName:
		return []evalValue{stringValue(event.Name())}
	case expression.EventFieldTime:
		return []evalValue{intValue(int64(event.Timestamp()))} //nolint:gosec // G115
	case expression.EventFieldTimeSinceStart:
		offset := event.Timestamp().AsTime().Sub(span.StartTimestamp().AsTime())
		return []evalValue{intValue(offset.Nanoseconds())}
	default:
		return nil
	}
}

func resolveLinkField(name string, link ptrace.SpanLink) []evalValue {
	switch name {
	case expression.LinkFieldTraceID:
		return []evalValue{stringValue(link.TraceID().String())}
	case expression.LinkFieldSpanID:
		return []evalValue{stringValue(link.SpanID().String())}
	case expression.LinkFieldTraceState:
		state := link.TraceState().AsRaw()
		if state == "" {
			return nil
		}
		return []evalValue{stringValue(state)}
	default:
		return nil
	}
}

func statusToWord(code ptrace.StatusCode) string {
	switch code {
	case ptrace.StatusCodeOk:
		return "ok"
	case ptrace.StatusCodeError:
		return "error"
	default:
		return "unset"
	}
}

// compareValues orders a and b. Every caller reaches it through resolveComparable, which
// guarantees comparableKinds(a, b) and that neither side is opaque or still untyped, so the only
// case this itself has to split beyond two integers is a mix of an integer and a double.
func compareValues(a, b evalValue) int {
	switch {
	case a.kind == kindInt && b.kind == kindInt:
		return cmp.Compare(a.numInt, b.numInt)
	case a.kind == kindBool && b.kind == kindBool:
		if a.boolean == b.boolean {
			return 0
		}
		if !a.boolean && b.boolean {
			return -1
		}
		return 1
	case a.kind == kindString && b.kind == kindString:
		return cmp.Compare(a.str, b.str)
	default:
		// A double on one or both sides. A double beyond int64's range is
		// beyond every integer; an integral double within it compares as an
		// integer, so an int64 above 2^53 keeps its precision; any other
		// double compares in floating point.
		if a.kind == kindInt && b.kind == kindDouble && (b.num >= 1<<63 || b.num < -(1<<63)) {
			return -int(math.Copysign(1, b.num))
		}
		if b.kind == kindInt && a.kind == kindDouble && (a.num >= 1<<63 || a.num < -(1<<63)) {
			return int(math.Copysign(1, a.num))
		}
		if a.kind == kindInt && isIntegral(b) {
			return cmp.Compare(a.numInt, int64(b.num))
		}
		if b.kind == kindInt && isIntegral(a) {
			return cmp.Compare(int64(a.num), b.numInt)
		}
		af, bf := a.num, b.num
		if a.kind == kindInt {
			af = float64(a.numInt)
		}
		if b.kind == kindInt {
			bf = float64(b.numInt)
		}
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}
}
