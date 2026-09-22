// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// newFilterFixture builds a span, in the context of a resource and scope,
// with enough shape (attributes at every level, an event and a link) for the
// filter tests below to exercise every level and operator without each test
// constructing its own trace from scratch.
type filterFixture struct {
	resource pcommon.Resource
	scope    pcommon.InstrumentationScope
	span     ptrace.Span
}

func newFilterFixture(t *testing.T) filterFixture {
	t.Helper()
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "checkout")
	rs.Resource().Attributes().PutStr("deployment.environment", "prod")

	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("otelgrpc")
	ss.Scope().SetVersion("1.2.3")
	ss.Scope().Attributes().PutStr("scope.tag", "scope-value")

	span := ss.Spans().AppendEmpty()
	span.SetTraceID(pcommon.TraceID{1})
	span.SetSpanID(pcommon.SpanID{1})
	span.SetName("POST /cart")
	span.SetKind(ptrace.SpanKindServer)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(150 * time.Millisecond)))
	span.Status().SetCode(ptrace.StatusCodeError)
	span.Status().SetMessage("boom")
	span.Attributes().PutStr("http.method", "POST")
	span.Attributes().PutInt("http.status_code", 500)
	span.Attributes().PutDouble("duration_ms", 150.5)
	span.Attributes().PutBool("retry", false)

	event := span.Events().AppendEmpty()
	event.SetName("exception")
	event.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(50 * time.Microsecond)))
	event.Attributes().PutStr("exception.type", "TimeoutError")

	other := span.Events().AppendEmpty()
	other.SetName("retry-scheduled")
	other.SetTimestamp(pcommon.NewTimestampFromTime(start.Add(10 * time.Microsecond)))

	link := span.Links().AppendEmpty()
	link.SetTraceID(pcommon.TraceID{2})
	link.SetSpanID(pcommon.SpanID{2})
	link.Attributes().PutStr("link.tag", "link-value")

	return filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: span}
}

func (f filterFixture) matches(filter *expression.Call) bool {
	return matchesFilter(filter, f.resource, f.scope, f.span)
}

func call(op expression.Operator, args ...expression.Expression) *expression.Call {
	return &expression.Call{Op: op, Args: args}
}

func fieldRef(level expression.Level, name string) *expression.FieldRef {
	return &expression.FieldRef{Level: level, Name: name}
}

func attrRef(level expression.Level, key string) *expression.AttributeRef {
	return &expression.AttributeRef{Level: level, Key: key}
}

func str(v string) *expression.StringValue          { return &expression.StringValue{Value: v} }
func intVal(v int64) *expression.IntValue           { return &expression.IntValue{Value: v} }
func dbl(v float64) *expression.DoubleValue         { return &expression.DoubleValue{Value: v} }
func boolean(v bool) *expression.BoolValue          { return &expression.BoolValue{Value: v} }
func dur(v time.Duration) *expression.DurationValue { return &expression.DurationValue{Value: v} }

func TestMatchesFilter_NilFilterMatchesEverything(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(nil))
}

func TestMatchesFilter_And(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpAnd,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("POST /cart")),
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("error")),
	)))
	assert.False(t, f.matches(call(expression.OpAnd,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("POST /cart")),
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("ok")),
	)))
}

func TestMatchesFilter_Or(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpOr,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("ok")),
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("error")),
	)))
	assert.False(t, f.matches(call(expression.OpOr,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("ok")),
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("unset")),
	)))
}

func TestMatchesFilter_Not(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpNot,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("ok")),
	)))
	assert.False(t, f.matches(call(expression.OpNot,
		call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("error")),
	)))
}

func TestMatchesFilter_SpanFields(t *testing.T) {
	f := newFilterFixture(t)
	tests := []struct {
		name string
		pred *expression.Call
		want bool
	}{
		{"traceID eq", call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldTraceID), str(f.span.TraceID().String())), true},
		{"spanID eq", call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldSpanID), str(f.span.SpanID().String())), true},
		{"parentSpanID absent", call(expression.OpExists, fieldRef(expression.LevelSpan, expression.SpanFieldParentSpanID)), false},
		{"name eq", call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("POST /cart")), true},
		{"name ne no match", call(expression.OpNe, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("POST /cart")), false},
		{"name ne match", call(expression.OpNe, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("GET /cart")), true},
		{"kind eq", call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldKind), str("server")), true},
		{"status eq", call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("error")), true},
		{"statusMessage eq", call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatusMessage), str("boom")), true},
		{"duration gt", call(expression.OpGt, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), dur(100*time.Millisecond)), true},
		{"duration lt", call(expression.OpLt, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), dur(100*time.Millisecond)), false},
		{"startTime exists", call(expression.OpExists, fieldRef(expression.LevelSpan, expression.SpanFieldStartTime)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, f.matches(tt.pred))
		})
	}
}

func TestMatchesFilter_ResourceAndScopeFields(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelResource, expression.ResourceFieldService), str("checkout"))))
	assert.False(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelResource, expression.ResourceFieldService), str("other"))))
	assert.True(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelScope, expression.ScopeFieldName), str("otelgrpc"))))
	assert.True(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelScope, expression.ScopeFieldVersion), str("1.2.3"))))
}

func TestMatchesFilter_Attributes(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "http.method"), str("POST"))))
	assert.True(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "http.status_code"), intVal(500))))
	assert.True(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "duration_ms"), dbl(150.5))))
	assert.True(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "retry"), boolean(false))))
	assert.True(t, f.matches(call(expression.OpEq, attrRef(expression.LevelResource, "deployment.environment"), str("prod"))))
	assert.True(t, f.matches(call(expression.OpEq, attrRef(expression.LevelScope, "scope.tag"), str("scope-value"))))
	assert.False(t, f.matches(call(expression.OpExists, attrRef(expression.LevelSpan, "nonexistent"))))
}

func TestMatchesFilter_UnqualifiedAttributeSearchesSpanAndResource(t *testing.T) {
	f := newFilterFixture(t)
	// http.method lives only on the span; deployment.environment only on the resource.
	// An unqualified (empty-level) attribute reference must find both.
	assert.True(t, f.matches(call(expression.OpEq, attrRef("", "http.method"), str("POST"))))
	assert.True(t, f.matches(call(expression.OpEq, attrRef("", "deployment.environment"), str("prod"))))
	assert.False(t, f.matches(call(expression.OpExists, attrRef("", "nope"))))
}

func TestMatchesFilter_Regex(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpRegex, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("cart$"))))
	assert.False(t, f.matches(call(expression.OpRegex, fieldRef(expression.LevelSpan, expression.SpanFieldName), str("^cart"))))
}

func TestMatchesFilter_InAndNotIn(t *testing.T) {
	f := newFilterFixture(t)
	list := &expression.List{Values: []string{"GET /cart", "POST /cart"}, Type: expression.ValueTypeString}
	assert.True(t, f.matches(call(expression.OpIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), list)))

	otherList := &expression.List{Values: []string{"GET /cart"}, Type: expression.ValueTypeString}
	assert.False(t, f.matches(call(expression.OpIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), otherList)))
	assert.True(t, f.matches(call(expression.OpNotIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), otherList)))

	// not_in on an absent reference is false, same as every other leaf comparison;
	// only a boolean `not` around it would make an absent reference match.
	assert.False(t, f.matches(call(expression.OpNotIn, fieldRef(expression.LevelSpan, expression.SpanFieldParentSpanID), otherList)))
}

func TestMatchesFilter_AbsentReferenceLeafComparisonsAreFalse(t *testing.T) {
	f := newFilterFixture(t)
	absent := fieldRef(expression.LevelSpan, expression.SpanFieldParentSpanID)
	assert.False(t, f.matches(call(expression.OpEq, absent, str("anything"))))
	assert.False(t, f.matches(call(expression.OpNe, absent, str("anything"))))
	assert.False(t, f.matches(call(expression.OpGt, absent, str("anything"))))
	assert.False(t, f.matches(call(expression.OpRegex, absent, str(".*"))))
	// Only `not` turns the absence into a match.
	assert.True(t, f.matches(call(expression.OpNot, call(expression.OpEq, absent, str("anything")))))
}

func TestMatchesFilter_EventUncorrelatedMatchesAnyElement(t *testing.T) {
	f := newFilterFixture(t)
	// Outside `some`, event.name and event fields from *different* events both
	// match independently (uncorrelated "any element" reading, RFC 0005 §5.5) —
	// even though no single event has both this name and this attribute.
	assert.True(t, f.matches(call(expression.OpAnd,
		call(expression.OpEq, fieldRef(expression.LevelEvent, expression.EventFieldName), str("retry-scheduled")),
		call(expression.OpEq, attrRef(expression.LevelEvent, "exception.type"), str("TimeoutError")),
	)))
}

func TestMatchesFilter_SomeCorrelatesASingleEvent(t *testing.T) {
	f := newFilterFixture(t)
	// Correlated: some single event named "exception" also carries exception.type.
	correlated := call(expression.OpSome, &expression.NestedRef{Level: expression.LevelEvent},
		call(expression.OpAnd,
			call(expression.OpEq, fieldRef(expression.LevelEvent, expression.EventFieldName), str("exception")),
			call(expression.OpEq, attrRef(expression.LevelEvent, "exception.type"), str("TimeoutError")),
		),
	)
	assert.True(t, f.matches(correlated))

	// No single event is named "retry-scheduled" and also carries exception.type.
	uncorrelated := call(expression.OpSome, &expression.NestedRef{Level: expression.LevelEvent},
		call(expression.OpAnd,
			call(expression.OpEq, fieldRef(expression.LevelEvent, expression.EventFieldName), str("retry-scheduled")),
			call(expression.OpEq, attrRef(expression.LevelEvent, "exception.type"), str("TimeoutError")),
		),
	)
	assert.False(t, f.matches(uncorrelated))
}

func TestMatchesFilter_SomeOverLinks(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpSome, &expression.NestedRef{Level: expression.LevelLink},
		call(expression.OpEq, attrRef(expression.LevelLink, "link.tag"), str("link-value")),
	)))
	assert.False(t, f.matches(call(expression.OpSome, &expression.NestedRef{Level: expression.LevelLink},
		call(expression.OpEq, attrRef(expression.LevelLink, "link.tag"), str("nope")),
	)))
}

func TestMatchesFilter_TimeSinceStart(t *testing.T) {
	f := newFilterFixture(t)
	// The "exception" event fires 50us after the span starts.
	assert.True(t, f.matches(call(expression.OpSome, &expression.NestedRef{Level: expression.LevelEvent},
		call(expression.OpAnd,
			call(expression.OpEq, fieldRef(expression.LevelEvent, expression.EventFieldName), str("exception")),
			call(expression.OpGt, fieldRef(expression.LevelEvent, expression.EventFieldTimeSinceStart), dur(40*time.Microsecond)),
		),
	)))
}

func TestMatchesFilter_UnknownOperatorDoesNotMatch(t *testing.T) {
	f := newFilterFixture(t)
	assert.False(t, f.matches(call(expression.Operator("made_up"), fieldRef(expression.LevelSpan, expression.SpanFieldName))))
}

func TestMatchesFilter_NonCallExpressionDoesNotMatch(t *testing.T) {
	f := newFilterFixture(t)
	assert.False(t, evalPredicate(str("not a call"), filterCtx{span: f.span}))
}

func TestMatchesFilter_LinkFields(t *testing.T) {
	f := newFilterFixture(t)
	link := f.span.Links().At(0)
	assert.True(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelLink, expression.LinkFieldTraceID), str(link.TraceID().String()))))
	assert.True(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelLink, expression.LinkFieldSpanID), str(link.SpanID().String()))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelLink, expression.LinkFieldTraceState))))
}

func TestMatchesFilter_StatusWords(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()

	okSpan := ss.Spans().AppendEmpty()
	okSpan.Status().SetCode(ptrace.StatusCodeOk)
	okFixture := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: okSpan}
	assert.True(t, okFixture.matches(call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("ok"))))

	unsetSpan := ss.Spans().AppendEmpty()
	unsetFixture := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: unsetSpan}
	assert.True(t, unsetFixture.matches(call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStatus), str("unset"))))
}

func TestMatchesFilter_ResourceAndScopeMissingFields(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	f := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: span}

	// No service.name resource attribute, no scope name/version, and neither
	// level has a schemaURL field this store can populate.
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelResource, expression.ResourceFieldService))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelResource, expression.ResourceFieldSchemaURL))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelScope, expression.ScopeFieldName))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelScope, expression.ScopeFieldVersion))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelScope, expression.ScopeFieldSchemaURL))))
}

func TestMatchesFilter_EventTimeField(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpSome, &expression.NestedRef{Level: expression.LevelEvent},
		call(expression.OpAnd,
			call(expression.OpEq, fieldRef(expression.LevelEvent, expression.EventFieldName), str("exception")),
			call(expression.OpExists, fieldRef(expression.LevelEvent, expression.EventFieldTime)),
		),
	)))
}

func TestMatchesFilter_InAndNotInAgainstNonList(t *testing.T) {
	f := newFilterFixture(t)
	notAList := str("not-a-list")
	assert.False(t, f.matches(call(expression.OpIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), notAList)))
	assert.False(t, f.matches(call(expression.OpNotIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), notAList)))
}

func TestMatchesFilter_InNumericAndBoolList(t *testing.T) {
	f := newFilterFixture(t)
	numList := &expression.List{Values: []string{"500", "404"}, Type: expression.ValueTypeInt}
	assert.True(t, f.matches(call(expression.OpIn, attrRef(expression.LevelSpan, "http.status_code"), numList)))

	boolList := &expression.List{Values: []string{"false"}, Type: expression.ValueTypeBool}
	assert.True(t, f.matches(call(expression.OpIn, attrRef(expression.LevelSpan, "retry"), boolList)))
}

func TestMatchesFilter_RegexInvalidPatternOperand(t *testing.T) {
	f := newFilterFixture(t)
	// The pattern operand must resolve to exactly one string value; a
	// reference that resolves to none (parentSpanID is absent on this fixture)
	// is refused rather than matched.
	assert.False(t, f.matches(call(expression.OpRegex, fieldRef(expression.LevelSpan, expression.SpanFieldName), fieldRef(expression.LevelSpan, expression.SpanFieldParentSpanID))))
}

func TestMatchesFilter_SomeOverUnknownLevelDoesNotMatch(t *testing.T) {
	f := newFilterFixture(t)
	assert.False(t, f.matches(call(expression.OpSome, &expression.NestedRef{Level: expression.LevelSpan},
		call(expression.OpExists, fieldRef(expression.LevelSpan, expression.SpanFieldName)),
	)))
}

func TestMatchesFilter_SomeWithNonNestedRefCollectionDoesNotMatch(t *testing.T) {
	f := newFilterFixture(t)
	assert.False(t, f.matches(call(expression.OpSome, str("not-a-collection"),
		call(expression.OpExists, fieldRef(expression.LevelEvent, expression.EventFieldName)),
	)))
}

func TestMatchesFilter_CompareValuesFallsBackToStringOnMixedTypes(t *testing.T) {
	// compareValues only has to order operands the query boundary already
	// confirmed are the same kind; asked to compare a bool against a number
	// anyway (which cannot happen through the public evaluator entry points),
	// it falls back to string comparison rather than panicking.
	assert.NotEqual(t, 0, compareValues(evalValue{isBool: true, boolean: true}, evalValue{isNumber: true, num: 1}))
}

func TestMatchesFilter_TraceStateAndEndTime(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.TraceState().FromRaw("congo=t61rcWkgMzE")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(start.Add(time.Second)))

	link := span.Links().AppendEmpty()
	link.TraceState().FromRaw("congo=t61rcWkgMzE")

	f := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: span}
	assert.True(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldTraceState), str("congo=t61rcWkgMzE"))))
	assert.True(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldEndTime), intVal(int64(span.EndTimestamp())))))
	assert.True(t, f.matches(call(expression.OpSome, &expression.NestedRef{Level: expression.LevelLink},
		call(expression.OpEq, fieldRef(expression.LevelLink, expression.LinkFieldTraceState), str("congo=t61rcWkgMzE")),
	)))
}

func TestCompareValues_BoolOrdering(t *testing.T) {
	trueVal := evalValue{isBool: true, boolean: true}
	falseVal := evalValue{isBool: true, boolean: false}
	assert.Equal(t, 0, compareValues(trueVal, trueVal))
	assert.Negative(t, compareValues(falseVal, trueVal))
	assert.Positive(t, compareValues(trueVal, falseVal))
}

func TestAttrToEvalValue_UnsupportedTypeIsSkipped(t *testing.T) {
	m := pcommon.NewMap()
	m.PutEmptyBytes("payload").FromRaw([]byte{1, 2, 3})
	v, ok := m.Get("payload")
	require.True(t, ok)
	_, resolved := attrToEvalValue(v)
	assert.False(t, resolved, "a bytes-valued attribute has no scalar reading to compare against")
}

func TestResolveOperand_NestedRefResolvesToNothingOutsideSome(t *testing.T) {
	f := newFilterFixture(t)
	values := resolveOperand(&expression.NestedRef{Level: expression.LevelEvent}, filterCtx{
		resource: f.resource, scope: f.scope, span: f.span,
	})
	assert.Empty(t, values)
}
