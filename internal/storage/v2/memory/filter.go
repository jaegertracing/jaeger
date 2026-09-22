// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"regexp"
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
type filterCtx struct {
	resource pcommon.Resource
	scope    pcommon.InstrumentationScope
	span     ptrace.Span

	boundEvent *ptrace.SpanEvent
	boundLink  *ptrace.SpanLink
}

// evalValue is a resolved operand: a constant from the filter or a value read
// off the span. Duration and timestamp values collapse into num (nanoseconds
// and UnixNano respectively) because ordering and equality only ever need the
// numeric form; the query boundary has already confirmed the two sides of a
// comparison hold the same kind of value before this filter reaches storage
// (RFC 0005 §5.3), so evalValue does not re-check compatibility.
type evalValue struct {
	isString bool
	isNumber bool
	isBool   bool
	str      string
	num      float64
	boolean  bool
}

// matchesFilter reports whether a span, in the context of its resource and
// scope, satisfies filter. A nil filter matches every span.
func matchesFilter(filter *expression.Call, resource pcommon.Resource, scope pcommon.InstrumentationScope, span ptrace.Span) bool {
	if filter == nil {
		return true
	}
	ctx := filterCtx{resource: resource, scope: scope, span: span}
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
		return anyPairMatches(call.Args[0], call.Args[1], ctx, valuesEqual)
	case expression.OpNe:
		return leafPresentAndNoPairMatches(call.Args[0], call.Args[1], ctx, valuesEqual)
	case expression.OpGt:
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(a, b evalValue) bool { return compareValues(a, b) > 0 })
	case expression.OpLt:
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(a, b evalValue) bool { return compareValues(a, b) < 0 })
	case expression.OpGte:
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(a, b evalValue) bool { return compareValues(a, b) >= 0 })
	case expression.OpLte:
		return anyPairMatches(call.Args[0], call.Args[1], ctx, func(a, b evalValue) bool { return compareValues(a, b) <= 0 })
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
// resolved values satisfies pred. Either side resolving to nothing (an
// absent reference) makes the predicate false, since a leaf comparison other
// than a boolean `not` never turns absence into a match.
func anyPairMatches(left, right expression.Expression, ctx filterCtx, pred func(a, b evalValue) bool) bool {
	lv := resolveOperand(left, ctx)
	rv := resolveOperand(right, ctx)
	for _, a := range lv {
		for _, b := range rv {
			if pred(a, b) {
				return true
			}
		}
	}
	return false
}

// leafPresentAndNoPairMatches implements the negated-leaf rule (RFC 0005
// §5.3) that `ne` and `not_in` share: the predicate holds only when the
// reference side is present (resolves to at least one value) and none of its
// values match. This is deliberately not "not(anyPairMatches(...))" — an
// absent reference must evaluate to false, the same as a positive leaf,
// and only a boolean `not` around the whole comparison flips that.
func leafPresentAndNoPairMatches(left, right expression.Expression, ctx filterCtx, pred func(a, b evalValue) bool) bool {
	lv := resolveOperand(left, ctx)
	rv := resolveOperand(right, ctx)
	if len(lv) == 0 || len(rv) == 0 {
		return false
	}
	for _, a := range lv {
		for _, b := range rv {
			if pred(a, b) {
				return false
			}
		}
	}
	return true
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

func valueInList(v evalValue, list *expression.List) bool {
	for _, elem := range list.Values {
		if v.isString && v.str == elem {
			return true
		}
		if v.isNumber {
			if n, err := strconv.ParseFloat(elem, 64); err == nil && v.num == n {
				return true
			}
		}
		if v.isBool {
			if b, err := strconv.ParseBool(elem); err == nil && v.boolean == b {
				return true
			}
		}
	}
	return false
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
		return []evalValue{{isNumber: true, num: float64(e.Value)}}
	case *expression.DoubleValue:
		return []evalValue{{isNumber: true, num: e.Value}}
	case *expression.BoolValue:
		return []evalValue{{isBool: true, boolean: e.Value}}
	case *expression.DurationValue:
		return []evalValue{{isNumber: true, num: float64(e.Value.Nanoseconds())}}
	case *expression.TimestampValue:
		return []evalValue{{isNumber: true, num: float64(e.Value.UnixNano())}}
	case *expression.AnyValue:
		return []evalValue{{isString: true, str: e.Value}}
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

func attrToEvalValue(v pcommon.Value) (evalValue, bool) {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return evalValue{isString: true, str: v.Str()}, true
	case pcommon.ValueTypeInt:
		return evalValue{isNumber: true, num: float64(v.Int())}, true
	case pcommon.ValueTypeDouble:
		return evalValue{isNumber: true, num: v.Double()}, true
	case pcommon.ValueTypeBool:
		return evalValue{isBool: true, boolean: v.Bool()}, true
	default:
		// Bytes, slice and map attributes have no scalar reading a filter
		// predicate can compare against.
		return evalValue{}, false
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
			if v, ok := attrToEvalValue(raw); ok {
				values = append(values, v)
			}
		}
	}
	return values
}

func resolveFieldRef(ref expression.FieldRef, ctx filterCtx) []evalValue {
	switch ref.Level {
	case expression.LevelSpan:
		return resolveSpanField(ref.Name, ctx.span)
	case expression.LevelResource:
		return resolveResourceField(ref.Name, ctx.resource)
	case expression.LevelScope:
		return resolveScopeField(ref.Name, ctx.scope)
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
		return []evalValue{{isNumber: true, num: float64(span.StartTimestamp())}}
	case expression.SpanFieldEndTime:
		return []evalValue{{isNumber: true, num: float64(span.EndTimestamp())}}
	case expression.SpanFieldDuration:
		dur := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
		return []evalValue{{isNumber: true, num: float64(dur.Nanoseconds())}}
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

func resolveResourceField(name string, resource pcommon.Resource) []evalValue {
	switch name {
	case expression.ResourceFieldService:
		svc := getServiceNameFromResource(resource)
		if svc == "" {
			return nil
		}
		return []evalValue{{isString: true, str: svc}}
	default:
		// ResourceFieldSchemaURL falls here too: pcommon.Resource carries no
		// schema URL of its own to read it from.
		return nil
	}
}

func resolveScopeField(name string, scope pcommon.InstrumentationScope) []evalValue {
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
	default:
		// ScopeFieldSchemaURL falls here too: pcommon.InstrumentationScope
		// carries no schema URL of its own to read it from.
		return nil
	}
}

func resolveEventField(name string, event ptrace.SpanEvent, span ptrace.Span) []evalValue {
	switch name {
	case expression.EventFieldName:
		return []evalValue{{isString: true, str: event.Name()}}
	case expression.EventFieldTime:
		return []evalValue{{isNumber: true, num: float64(event.Timestamp())}}
	case expression.EventFieldTimeSinceStart:
		offset := event.Timestamp().AsTime().Sub(span.StartTimestamp().AsTime())
		return []evalValue{{isNumber: true, num: float64(offset.Nanoseconds())}}
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

func valuesEqual(a, b evalValue) bool {
	return compareValues(a, b) == 0
}

// compareValues orders a and b. It is only ever asked to compare values the
// query boundary has already confirmed are the same kind (RFC 0005 §5.3), so
// it does not itself refuse a type mismatch; asked to anyway, it falls back
// to string comparison of whichever representation each side has, rather
// than panicking or silently reporting equal.
func compareValues(a, b evalValue) int {
	switch {
	case a.isNumber && b.isNumber:
		switch {
		case a.num < b.num:
			return -1
		case a.num > b.num:
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
	default:
		as, bs := a.str, b.str
		if a.isNumber {
			as = strconv.FormatFloat(a.num, 'f', -1, 64)
		}
		if b.isNumber {
			bs = strconv.FormatFloat(b.num, 'f', -1, 64)
		}
		switch {
		case as < bs:
			return -1
		case as > bs:
			return 1
		default:
			return 0
		}
	}
}
