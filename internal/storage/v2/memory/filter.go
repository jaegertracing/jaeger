// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"regexp"
	"slices"
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
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
}

// evalValue is a resolved operand: a constant from the filter or a value read
// off the span. Exactly one of isString, isInt, isNumber, isBool, isOpaque or
// isUntyped is set, except that isUntyped is resolved away (into one of the
// others) before compareValues or valueInList ever sees it — see
// resolveComparable and coerceUntyped.
//
// isInt and isNumber are kept apart, rather than folded into one float64
// field, because an integer read as float64 loses precision once it passes
// 2^53: two adjacent current-era nanosecond timestamps, a millisecond apart
// in wall-clock terms but one nanosecond apart in their stored int64, would
// otherwise compare equal. isInt carries every exact integer — an IntValue
// constant, a Duration or Timestamp constant resolved to nanoseconds, an
// int-valued attribute, and every span/event time or duration field — at
// full int64 precision. isNumber is only ever a DoubleValue constant or a
// double-valued attribute, which are floating-point by nature and have no
// exact form to preserve.
//
// isOpaque marks a present attribute (bytes, slice or map — the OTLP types a
// filter predicate has no scalar reading for) that resolveAttributeRef still
// has to report as present, since OpExists checks presence rather than
// comparability. It is never comparable: resolveComparable refuses any pair
// that includes one.
//
// isUntyped marks an AnyValue constant, which the caller wrote under no type
// constraint (RFC 0005 §5.4). Its str field holds the raw text as written;
// resolveComparable resolves that text against the paired operand's actual
// kind before a comparison runs, rather than treating it as a string, which
// is what made an untyped `"500"` fail to match a numeric or boolean
// attribute that happened to hold that value.
type evalValue struct {
	isString  bool
	isInt     bool
	isNumber  bool
	isBool    bool
	isOpaque  bool
	isUntyped bool
	str       string
	numInt    int64
	num       float64
	boolean   bool
}

// evalKind is the comparable shape a resolved evalValue has, once isUntyped
// is resolved away. Two values compare only when they share a kind:
// kindNumber covers both isInt and isNumber, since an int and a double are
// still the same broad kind of thing to compare, unlike a string or a bool.
type evalKind int

const (
	kindNone evalKind = iota
	kindString
	kindNumber
	kindBool
)

func (v evalValue) kind() evalKind {
	switch {
	case v.isString:
		return kindString
	case v.isBool:
		return kindBool
	case v.isInt || v.isNumber:
		return kindNumber
	default:
		// isOpaque, or isUntyped not yet resolved against anything.
		return kindNone
	}
}

// matchesFilter reports whether a span, in the context of its resource and
// scope, satisfies filter. A nil filter matches every span.
func matchesFilter(
	filter *expression.Call,
	resource pcommon.Resource,
	scope pcommon.InstrumentationScope,
	span ptrace.Span,
	resourceSchemaURL, scopeSchemaURL string,
) bool {
	if filter == nil {
		return true
	}
	ctx := filterCtx{
		resource: resource, scope: scope, span: span,
		resourceSchemaURL: resourceSchemaURL, scopeSchemaURL: scopeSchemaURL,
	}
	return evalPredicate(filter, ctx)
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
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c > 0 })
	case expression.OpLt:
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c < 0 })
	case expression.OpGte:
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c >= 0 })
	case expression.OpLte:
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(c int) bool { return c <= 0 })
	case expression.OpRegex:
		return evalRegex(call.Args[0], call.Args[1], ctx)
	case expression.OpIn:
		return evalIn(call.Args[0], call.Args[1], ctx)
	case expression.OpNotIn:
		values := resolveOperand(call.Args[0], ctx)
		if len(values) == 0 {
			return false
		}
		list, ok := call.Args[1].(*expression.List)
		if !ok {
			return false
		}
		for _, v := range values {
			if valueInList(v, list) {
				return false
			}
		}
		return true
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
func anyPairMatches(left, right expression.Expression, ctx filterCtx, test func(cmp int) bool) bool {
	lv := resolveOperand(left, ctx)
	rv := resolveOperand(right, ctx)
	for _, a := range lv {
		for _, b := range rv {
			ca, cb, ok := resolveComparable(a, b)
			if ok && test(compareValues(ca, cb)) {
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
func leafPresentAndNoPairMatches(left, right expression.Expression, ctx filterCtx, test func(cmp int) bool) bool {
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
// It resolves either side's isUntyped value (an AnyValue) against the
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
	if a.isOpaque || b.isOpaque {
		return evalValue{}, evalValue{}, false
	}
	if a.isUntyped {
		resolved, ok := coerceUntyped(a, b)
		if !ok {
			return evalValue{}, evalValue{}, false
		}
		a = resolved
	}
	if b.isUntyped {
		resolved, ok := coerceUntyped(b, a)
		if !ok {
			return evalValue{}, evalValue{}, false
		}
		b = resolved
	}
	if a.kind() == kindNone || a.kind() != b.kind() {
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
	switch other.kind() {
	case kindBool:
		b, err := strconv.ParseBool(v.str)
		if err != nil {
			return evalValue{}, false
		}
		return evalValue{isBool: true, boolean: b}, true
	case kindNumber:
		if n, err := strconv.ParseInt(v.str, 10, 64); err == nil {
			return evalValue{isInt: true, numInt: n}, true
		}
		f, err := strconv.ParseFloat(v.str, 64)
		if err != nil {
			return evalValue{}, false
		}
		return evalValue{isNumber: true, num: f}, true
	default:
		// kindString, or kindNone (other is itself opaque — excluded by
		// resolveComparable before this is called — or still untyped, in
		// which case it resolves against v's own text next and the pair
		// ends up compared as raw strings).
		return evalValue{isString: true, str: v.str}, true
	}
}

func evalRegex(ref, pattern expression.Expression, ctx filterCtx) bool {
	values := resolveOperand(ref, ctx)
	if len(values) == 0 {
		return false
	}
	patternValues := resolveOperand(pattern, ctx)
	if len(patternValues) != 1 || !patternValues[0].isString {
		return false
	}
	re, err := regexp.Compile(patternValues[0].str)
	if err != nil {
		// The query boundary parses the pattern before a backend ever sees it
		// (RFC 0005 §5.3), so this is unreached in practice; refusing to match
		// is the safe fallback if it somehow is not.
		return false
	}
	for _, v := range values {
		if v.isString && re.MatchString(v.str) {
			return true
		}
	}
	return false
}

func evalIn(ref, listExpr expression.Expression, ctx filterCtx) bool {
	values := resolveOperand(ref, ctx)
	if len(values) == 0 {
		return false
	}
	list, ok := listExpr.(*expression.List)
	if !ok {
		return false
	}
	for _, v := range values {
		if valueInList(v, list) {
			return true
		}
	}
	return false
}

// valueInList reports whether v matches one of list's elements. list.Type,
// when set, is authoritative (RFC 0005 §5.4): only a value of that declared
// kind may match, and each element is parsed as that kind — a numeric
// attribute does not match a string-typed list containing its digits, and a
// string attribute does not match an int-typed list the same way. An empty
// Type means the elements are read at whatever kind v itself resolved to,
// the same way an untyped scalar beside an attribute is (coerceUntyped):
// neither the list nor an attribute reference has a static type to supply
// one otherwise.
func valueInList(v evalValue, list *expression.List) bool {
	kind := list.Type
	if kind == "" {
		switch {
		case v.isString:
			kind = expression.ValueTypeString
		case v.isInt:
			kind = expression.ValueTypeInt
		case v.isNumber:
			kind = expression.ValueTypeDouble
		case v.isBool:
			kind = expression.ValueTypeBool
		default:
			// Opaque or still-untyped: nothing to match at any kind.
			return false
		}
	}
	switch kind {
	case expression.ValueTypeString:
		return v.isString && slices.Contains(list.Values, v.str)
	case expression.ValueTypeInt:
		if !v.isInt {
			return false
		}
		for _, elem := range list.Values {
			if n, err := strconv.ParseInt(elem, 10, 64); err == nil && v.numInt == n {
				return true
			}
		}
		return false
	case expression.ValueTypeDouble:
		if !v.isNumber {
			return false
		}
		for _, elem := range list.Values {
			if n, err := strconv.ParseFloat(elem, 64); err == nil && v.num == n {
				return true
			}
		}
		return false
	case expression.ValueTypeBool:
		if !v.isBool {
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
		return []evalValue{{isString: true, str: e.Value}}
	case *expression.IntValue:
		return []evalValue{{isInt: true, numInt: e.Value}}
	case *expression.DoubleValue:
		return []evalValue{{isNumber: true, num: e.Value}}
	case *expression.BoolValue:
		return []evalValue{{isBool: true, boolean: e.Value}}
	case *expression.DurationValue:
		return []evalValue{{isInt: true, numInt: e.Value.Nanoseconds()}}
	case *expression.TimestampValue:
		return []evalValue{{isInt: true, numInt: e.Value.UnixNano()}}
	case *expression.AnyValue:
		return []evalValue{{isUntyped: true, str: e.Value}}
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
		return evalValue{isString: true, str: v.Str()}
	case pcommon.ValueTypeInt:
		return evalValue{isInt: true, numInt: v.Int()}
	case pcommon.ValueTypeDouble:
		return evalValue{isNumber: true, num: v.Double()}
	case pcommon.ValueTypeBool:
		return evalValue{isBool: true, boolean: v.Bool()}
	default:
		return evalValue{isOpaque: true}
	}
}

func resolveAttributeRef(ref expression.AttributeRef, ctx filterCtx) []evalValue {
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
		// Empty level: the unqualified span-or-resource search (RFC 0005 §5.1).
		maps = []pcommon.Map{ctx.span.Attributes(), ctx.resource.Attributes()}
	}
	var values []evalValue
	for _, m := range maps {
		if raw, ok := m.Get(ref.Key); ok {
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
		return []evalValue{{isString: true, str: span.TraceID().String()}}
	case expression.SpanFieldSpanID:
		return []evalValue{{isString: true, str: span.SpanID().String()}}
	case expression.SpanFieldParentSpanID:
		if span.ParentSpanID().IsEmpty() {
			return nil
		}
		return []evalValue{{isString: true, str: span.ParentSpanID().String()}}
	case expression.SpanFieldTraceState:
		state := span.TraceState().AsRaw()
		if state == "" {
			return nil
		}
		return []evalValue{{isString: true, str: state}}
	case expression.SpanFieldName:
		return []evalValue{{isString: true, str: span.Name()}}
	case expression.SpanFieldKind:
		return []evalValue{{isString: true, str: fromOTELSpanKind(span.Kind())}}
	case expression.SpanFieldStartTime:
		return []evalValue{{isInt: true, numInt: int64(span.StartTimestamp())}} //nolint:gosec // G115
	case expression.SpanFieldEndTime:
		return []evalValue{{isInt: true, numInt: int64(span.EndTimestamp())}} //nolint:gosec // G115
	case expression.SpanFieldDuration:
		dur := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
		return []evalValue{{isInt: true, numInt: dur.Nanoseconds()}}
	case expression.SpanFieldStatus:
		return []evalValue{{isString: true, str: statusToWord(span.Status().Code())}}
	case expression.SpanFieldStatusMessage:
		if span.Status().Message() == "" {
			return nil
		}
		return []evalValue{{isString: true, str: span.Status().Message()}}
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
		return []evalValue{{isString: true, str: svc}}
	case expression.ResourceFieldSchemaURL:
		if schemaURL == "" {
			return nil
		}
		return []evalValue{{isString: true, str: schemaURL}}
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
		return []evalValue{{isString: true, str: scope.Name()}}
	case expression.ScopeFieldVersion:
		if scope.Version() == "" {
			return nil
		}
		return []evalValue{{isString: true, str: scope.Version()}}
	case expression.ScopeFieldSchemaURL:
		if schemaURL == "" {
			return nil
		}
		return []evalValue{{isString: true, str: schemaURL}}
	default:
		return nil
	}
}

func resolveEventField(name string, event ptrace.SpanEvent, span ptrace.Span) []evalValue {
	switch name {
	case expression.EventFieldName:
		return []evalValue{{isString: true, str: event.Name()}}
	case expression.EventFieldTime:
		return []evalValue{{isInt: true, numInt: int64(event.Timestamp())}} //nolint:gosec // G115
	case expression.EventFieldTimeSinceStart:
		offset := event.Timestamp().AsTime().Sub(span.StartTimestamp().AsTime())
		return []evalValue{{isInt: true, numInt: offset.Nanoseconds()}}
	default:
		return nil
	}
}

func resolveLinkField(name string, link ptrace.SpanLink) []evalValue {
	switch name {
	case expression.LinkFieldTraceID:
		return []evalValue{{isString: true, str: link.TraceID().String()}}
	case expression.LinkFieldSpanID:
		return []evalValue{{isString: true, str: link.SpanID().String()}}
	case expression.LinkFieldTraceState:
		state := link.TraceState().AsRaw()
		if state == "" {
			return nil
		}
		return []evalValue{{isString: true, str: state}}
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

// compareValues orders a and b. Every caller reaches it through
// resolveComparable, which guarantees a.kind() == b.kind() and neither side
// is opaque or still untyped — so the only case this itself has to split
// beyond isInt/isInt is a mix of isInt and isNumber, both being kindNumber.
func compareValues(a, b evalValue) int {
	switch {
	case a.isInt && b.isInt:
		switch {
		case a.numInt < b.numInt:
			return -1
		case a.numInt > b.numInt:
			return 1
		default:
			return 0
		}
	case a.isBool && b.isBool:
		if a.boolean == b.boolean {
			return 0
		}
		if !a.boolean && b.boolean {
			return -1
		}
		return 1
	case a.isString && b.isString:
		switch {
		case a.str < b.str:
			return -1
		case a.str > b.str:
			return 1
		default:
			return 0
		}
	default:
		// A double on one or both sides. An exact int64 compared against a
		// double loses precision only here, which is inherent to comparing
		// an integer against a floating-point value at all, not something
		// this function can avoid while still comparing the two at all.
		af, bf := a.num, b.num
		if a.isInt {
			af = float64(a.numInt)
		}
		if b.isInt {
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
