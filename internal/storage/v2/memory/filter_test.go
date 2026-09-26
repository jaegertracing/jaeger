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
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// newFilterFixture builds a span, in the context of a resource and scope,
// with enough shape (attributes at every level, an event and a link) for the
// filter tests below to exercise every level and operator without each test
// constructing its own trace from scratch.
type filterFixture struct {
	resource          pcommon.Resource
	scope             pcommon.InstrumentationScope
	span              ptrace.Span
	resourceSchemaURL string
	scopeSchemaURL    string
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
	return matchesFilter(filter, f.resource, f.scope, f.span, f.resourceSchemaURL, f.scopeSchemaURL)
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
	// not_in is false when the value is found in the list, same list that made in true.
	assert.False(t, f.matches(call(expression.OpNotIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), list)))

	otherList := &expression.List{Values: []string{"GET /cart"}, Type: expression.ValueTypeString}
	assert.False(t, f.matches(call(expression.OpIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), otherList)))
	assert.True(t, f.matches(call(expression.OpNotIn, fieldRef(expression.LevelSpan, expression.SpanFieldName), otherList)))

	// not_in on an absent reference is false, same as every other leaf comparison;
	// only a boolean `not` around it would make an absent reference match.
	assert.False(t, f.matches(call(expression.OpNotIn, fieldRef(expression.LevelSpan, expression.SpanFieldParentSpanID), otherList)))
}

func TestMatchesFilter_GteLte(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpGte, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), dur(150*time.Millisecond))))
	assert.True(t, f.matches(call(expression.OpGte, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), dur(100*time.Millisecond))))
	assert.False(t, f.matches(call(expression.OpGte, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), dur(200*time.Millisecond))))

	assert.True(t, f.matches(call(expression.OpLte, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), dur(150*time.Millisecond))))
	assert.True(t, f.matches(call(expression.OpLte, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), dur(200*time.Millisecond))))
	assert.False(t, f.matches(call(expression.OpLte, fieldRef(expression.LevelSpan, expression.SpanFieldDuration), dur(100*time.Millisecond))))
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

	// No service.name resource attribute, no scope name/version, and this
	// fixture's ResourceSpans/ScopeSpans carry no schema URL either.
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelResource, expression.ResourceFieldService))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelResource, expression.ResourceFieldSchemaURL))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelScope, expression.ScopeFieldName))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelScope, expression.ScopeFieldVersion))))
	assert.False(t, f.matches(call(expression.OpExists, fieldRef(expression.LevelScope, expression.ScopeFieldSchemaURL))))
}

// TestMatchesFilter_SchemaURL pins that resource.schemaURL and scope.schemaURL actually
// resolve when the enclosing ResourceSpans/ScopeSpans carry one: pcommon.Resource and
// pcommon.InstrumentationScope have no schema URL field of their own to read it from, so
// matchesFilter's caller has to supply it separately.
func TestMatchesFilter_SchemaURL(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.SetSchemaUrl("https://opentelemetry.io/schemas/1.9.0")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.SetSchemaUrl("https://opentelemetry.io/schemas/1.4.0")
	span := ss.Spans().AppendEmpty()
	f := filterFixture{
		resource: rs.Resource(), scope: ss.Scope(), span: span,
		resourceSchemaURL: rs.SchemaUrl(), scopeSchemaURL: ss.SchemaUrl(),
	}

	assert.True(t, f.matches(call(expression.OpEq,
		fieldRef(expression.LevelResource, expression.ResourceFieldSchemaURL), str("https://opentelemetry.io/schemas/1.9.0"))))
	assert.False(t, f.matches(call(expression.OpEq,
		fieldRef(expression.LevelResource, expression.ResourceFieldSchemaURL), str("other"))))
	assert.True(t, f.matches(call(expression.OpEq,
		fieldRef(expression.LevelScope, expression.ScopeFieldSchemaURL), str("https://opentelemetry.io/schemas/1.4.0"))))
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

// TestMatchesFilter_ExplicitlyTypedConstantDoesNotCrossKind pins that a constant with an
// explicit type (StringValue, IntValue, BoolValue, ...) only ever matches an attribute of the
// same kind: RFC 0005 §5.4's "the query boundary intentionally leaves attribute types for
// storage to resolve" does not mean an explicitly typed string should match a numeric
// attribute that happens to render the same digits.
func TestMatchesFilter_ExplicitlyTypedConstantDoesNotCrossKind(t *testing.T) {
	f := newFilterFixture(t)
	// http.status_code is stored as an int attribute (500).
	assert.False(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "http.status_code"), str("500"))))
	assert.True(t, f.matches(call(expression.OpNe, attrRef(expression.LevelSpan, "http.status_code"), str("500"))),
		"present but a different kind, so ne is true rather than the leaf-absence false")
}

// TestMatchesFilter_UntypedConstantResolvesAgainstAttributeKind pins the other half of the
// same fix: an AnyValue constant (the caller wrote no type) resolves against whichever kind
// the paired attribute actually turns out to hold, rather than being read as a string.
func TestMatchesFilter_UntypedConstantResolvesAgainstAttributeKind(t *testing.T) {
	f := newFilterFixture(t)
	// http.status_code (int 500) and retry (bool false) beside an untyped "500"/"false".
	assert.True(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "http.status_code"), &expression.AnyValue{Value: "500"})))
	assert.True(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "retry"), &expression.AnyValue{Value: "false"})))
	// An untyped value that cannot be read as the attribute's kind matches nothing.
	assert.False(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "http.status_code"), &expression.AnyValue{Value: "not-a-number"})))
}

// TestMatchesFilter_OpaqueAttributeExistsButNeverCompares pins OpExists to presence alone: a
// bytes-valued attribute has no scalar reading, but it is still present, so exists must be
// true while eq/gt/lt, which need a scalar, never match it — and ne, which only needs presence
// and non-equality, is true.
func TestMatchesFilter_OpaqueAttributeExistsButNeverCompares(t *testing.T) {
	f := newFilterFixture(t)
	f.span.Attributes().PutEmptyBytes("payload").FromRaw([]byte{1, 2, 3})

	assert.True(t, f.matches(call(expression.OpExists, attrRef(expression.LevelSpan, "payload"))))
	assert.False(t, f.matches(call(expression.OpEq, attrRef(expression.LevelSpan, "payload"), str("anything"))))
	assert.True(t, f.matches(call(expression.OpNe, attrRef(expression.LevelSpan, "payload"), str("anything"))))
}

// TestMatchesFilter_IntPrecisionAtNanosecondTimestamps pins that comparing timestamps does
// not go through float64: two adjacent current-era nanosecond timestamps would collapse to the
// same float64 and compare equal if evalValue carried them as a double.
func TestMatchesFilter_IntPrecisionAtNanosecondTimestamps(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	const nanos = int64(1700000000000000001)
	span.SetStartTimestamp(pcommon.Timestamp(nanos))
	span.SetEndTimestamp(pcommon.Timestamp(nanos))
	f := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: span}

	assert.True(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStartTime), intVal(nanos))))
	assert.False(t, f.matches(call(expression.OpEq, fieldRef(expression.LevelSpan, expression.SpanFieldStartTime), intVal(nanos-1))))
}

// TestMatchesFilter_ListTypeIsAuthoritative pins RFC 0005 §5.4: a typed list matches only
// values of its declared kind, so a numeric attribute does not match a string-typed list that
// happens to contain its digits, and vice versa.
func TestMatchesFilter_ListTypeIsAuthoritative(t *testing.T) {
	f := newFilterFixture(t)
	// http.status_code is an int attribute (500).
	stringList := &expression.List{Values: []string{"500"}, Type: expression.ValueTypeString}
	assert.False(t, f.matches(call(expression.OpIn, attrRef(expression.LevelSpan, "http.status_code"), stringList)))

	// http.method is a string attribute ("POST"), never matches an int-typed list.
	intList := &expression.List{Values: []string{"500"}, Type: expression.ValueTypeInt}
	assert.False(t, f.matches(call(expression.OpIn, attrRef(expression.LevelSpan, "http.method"), intList)))
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

func TestEvalValueKind(t *testing.T) {
	assert.Equal(t, kindString, evalValue{isString: true}.kind())
	assert.Equal(t, kindBool, evalValue{isBool: true}.kind())
	assert.Equal(t, kindNumber, evalValue{isInt: true}.kind())
	assert.Equal(t, kindNumber, evalValue{isNumber: true}.kind())
	assert.Equal(t, kindNone, evalValue{isOpaque: true}.kind())
	assert.Equal(t, kindNone, evalValue{isUntyped: true, str: "x"}.kind())
}

func TestCoerceUntyped(t *testing.T) {
	tests := []struct {
		name    string
		v       evalValue
		other   evalValue
		wantOK  bool
		wantVal evalValue
	}{
		{"reads as bool", evalValue{isUntyped: true, str: "true"}, evalValue{isBool: true}, true, evalValue{isBool: true, boolean: true}},
		{"not a bool", evalValue{isUntyped: true, str: "nope"}, evalValue{isBool: true}, false, evalValue{}},
		{"reads as int", evalValue{isUntyped: true, str: "500"}, evalValue{isInt: true}, true, evalValue{isInt: true, numInt: 500}},
		{"reads as double when not an exact int", evalValue{isUntyped: true, str: "1.5"}, evalValue{isNumber: true}, true, evalValue{isNumber: true, num: 1.5}},
		{"not a number", evalValue{isUntyped: true, str: "nope"}, evalValue{isInt: true}, false, evalValue{}},
		{"reads as string against a string", evalValue{isUntyped: true, str: "hi"}, evalValue{isString: true}, true, evalValue{isString: true, str: "hi"}},
		{"reads as string against another untyped value", evalValue{isUntyped: true, str: "hi"}, evalValue{isUntyped: true, str: "hi"}, true, evalValue{isString: true, str: "hi"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := coerceUntyped(tt.v, tt.other)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantVal, got)
			}
		})
	}
}

func TestResolveComparable(t *testing.T) {
	t.Run("opaque on either side is never comparable", func(t *testing.T) {
		_, _, ok := resolveComparable(evalValue{isOpaque: true}, evalValue{isString: true, str: "x"})
		assert.False(t, ok)
		_, _, ok = resolveComparable(evalValue{isString: true, str: "x"}, evalValue{isOpaque: true})
		assert.False(t, ok)
	})
	t.Run("untyped left resolves against typed right", func(t *testing.T) {
		a, b, ok := resolveComparable(evalValue{isUntyped: true, str: "500"}, evalValue{isInt: true, numInt: 500})
		require.True(t, ok)
		assert.Equal(t, evalValue{isInt: true, numInt: 500}, a)
		assert.Equal(t, evalValue{isInt: true, numInt: 500}, b)
	})
	t.Run("untyped right resolves against typed left", func(t *testing.T) {
		a, b, ok := resolveComparable(evalValue{isBool: true, boolean: true}, evalValue{isUntyped: true, str: "true"})
		require.True(t, ok)
		assert.Equal(t, evalValue{isBool: true, boolean: true}, a)
		assert.Equal(t, evalValue{isBool: true, boolean: true}, b)
	})
	t.Run("untyped left fails to coerce", func(t *testing.T) {
		_, _, ok := resolveComparable(evalValue{isUntyped: true, str: "nope"}, evalValue{isBool: true})
		assert.False(t, ok)
	})
	t.Run("untyped right fails to coerce", func(t *testing.T) {
		_, _, ok := resolveComparable(evalValue{isInt: true}, evalValue{isUntyped: true, str: "nope"})
		assert.False(t, ok)
	})
	t.Run("mismatched kinds are never comparable", func(t *testing.T) {
		_, _, ok := resolveComparable(evalValue{isString: true, str: "x"}, evalValue{isBool: true})
		assert.False(t, ok)
	})
}

func TestValueInList_EmptyTypeInfersFromOperand(t *testing.T) {
	assert.True(t, valueInList(evalValue{isInt: true, numInt: 500}, &expression.List{Values: []string{"500"}}))
	assert.True(t, valueInList(evalValue{isNumber: true, num: 1.5}, &expression.List{Values: []string{"1.5"}}))
	assert.True(t, valueInList(evalValue{isBool: true, boolean: true}, &expression.List{Values: []string{"true"}}))
	assert.False(t, valueInList(evalValue{isOpaque: true}, &expression.List{Values: []string{"anything"}}),
		"opaque has no kind to infer, so it never matches an untyped list")
}

func TestValueInList_ExplicitDoubleType(t *testing.T) {
	list := &expression.List{Values: []string{"1.5", "2.5"}, Type: expression.ValueTypeDouble}
	assert.True(t, valueInList(evalValue{isNumber: true, num: 1.5}, list))
	assert.False(t, valueInList(evalValue{isNumber: true, num: 3.5}, list))
	assert.False(t, valueInList(evalValue{isInt: true, numInt: 1}, list), "a typed double list does not fall back to int")
}

func TestAttrToEvalValue_UnsupportedTypeIsOpaque(t *testing.T) {
	m := pcommon.NewMap()
	m.PutEmptyBytes("payload").FromRaw([]byte{1, 2, 3})
	v, ok := m.Get("payload")
	require.True(t, ok)
	resolved := attrToEvalValue(v)
	assert.True(t, resolved.isOpaque, "a bytes-valued attribute has no scalar reading, but is still present")
}

func TestResolveOperand_NestedRefResolvesToNothingOutsideSome(t *testing.T) {
	f := newFilterFixture(t)
	values := resolveOperand(&expression.NestedRef{Level: expression.LevelEvent}, filterCtx{
		resource: f.resource, scope: f.scope, span: f.span,
	})
	assert.Empty(t, values)
}

// TestMatchesFilter_ErrorVirtualAttribute pins that error is derived from the span's status
// rather than read as a literal attribute, at both the unqualified and span level, matching the
// legacy Attributes-based search and the elasticsearch backend's own error-tag special case.
func TestMatchesFilter_ErrorVirtualAttribute(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()

	errorSpan := ss.Spans().AppendEmpty()
	errorSpan.Status().SetCode(ptrace.StatusCodeError)
	errorFixture := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: errorSpan}

	okSpan := ss.Spans().AppendEmpty()
	okSpan.Status().SetCode(ptrace.StatusCodeOk)
	okFixture := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: okSpan}

	unsetSpan := ss.Spans().AppendEmpty()
	unsetFixture := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: unsetSpan}

	for _, level := range []expression.Level{"", expression.LevelSpan} {
		assert.True(t, errorFixture.matches(call(expression.OpEq, attrRef(level, errorAttribute), boolean(true))),
			"level=%q: an Error-status span matches error=true", level)
		assert.False(t, errorFixture.matches(call(expression.OpEq, attrRef(level, errorAttribute), boolean(false))),
			"level=%q: an Error-status span does not match error=false", level)
		// error=false is the complement of error=true, not "tag absent": Unset is by far the
		// common case and must match it, the same way the legacy search treats it.
		assert.True(t, okFixture.matches(call(expression.OpEq, attrRef(level, errorAttribute), boolean(false))),
			"level=%q: an Ok-status span matches error=false", level)
		assert.True(t, unsetFixture.matches(call(expression.OpEq, attrRef(level, errorAttribute), boolean(false))),
			"level=%q: an Unset-status span matches error=false, not just Ok", level)
		assert.True(t, errorFixture.matches(call(expression.OpExists, attrRef(level, errorAttribute))),
			"level=%q: error always exists, whether or not the span carries the attribute literally", level)
		assert.True(t, okFixture.matches(call(expression.OpNe, attrRef(level, errorAttribute), boolean(true))),
			"level=%q: ne is the leaf-present complement, same as any other attribute", level)
	}

	// A literal "error" attribute set on the span is shadowed by the virtual one: the span's
	// actual status still decides, not whatever the attribute map happens to hold.
	shadowed := ss.Spans().AppendEmpty()
	shadowed.Status().SetCode(ptrace.StatusCodeOk)
	shadowed.Attributes().PutBool(errorAttribute, true)
	shadowedFixture := filterFixture{resource: rs.Resource(), scope: ss.Scope(), span: shadowed}
	assert.True(t, shadowedFixture.matches(call(expression.OpEq, attrRef("", errorAttribute), boolean(false))),
		"the literal attribute value is shadowed by the status-derived one")

	// At any other level, "error" is an ordinary attribute lookup, not the virtual field: the
	// special case is specific to the span's own status.
	resourceErrorFixture := newFilterFixture(t)
	resourceErrorFixture.resource.Attributes().PutBool(errorAttribute, true)
	assert.True(t, resourceErrorFixture.matches(call(expression.OpEq, attrRef(expression.LevelResource, errorAttribute), boolean(true))),
		"resource-level error is a literal attribute, not the span-status virtual one")
}

// TestValidateFilterShape_Valid pins that every operator this evaluator supports, given the
// argument count and shape it expects, passes shape validation.
func TestValidateFilterShape_Valid(t *testing.T) {
	nameRef := fieldRef(expression.LevelSpan, expression.SpanFieldName)
	tests := []struct {
		name   string
		filter *expression.Call
	}{
		{"nil filter", nil},
		{"and with one arg", call(expression.OpAnd, call(expression.OpExists, nameRef))},
		{"or with several args", call(expression.OpOr,
			call(expression.OpExists, nameRef), call(expression.OpEq, nameRef, str("x")))},
		{"not", call(expression.OpNot, call(expression.OpExists, nameRef))},
		{"exists", call(expression.OpExists, nameRef)},
		{"eq", call(expression.OpEq, nameRef, str("x"))},
		{"regex", call(expression.OpRegex, nameRef, str("^x$"))},
		{"in", call(expression.OpIn, nameRef, &expression.List{Values: []string{"x"}})},
		{"some over events", call(expression.OpSome,
			&expression.NestedRef{Level: expression.LevelEvent},
			call(expression.OpExists, fieldRef(expression.LevelEvent, "name")))},
		{"nested and/or/not", call(expression.OpAnd,
			call(expression.OpOr, call(expression.OpExists, nameRef)),
			call(expression.OpNot, call(expression.OpExists, nameRef)))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, validateFilterShape(tt.filter))
		})
	}
}

// TestValidateFilterShape_Invalid pins that a filter a remote-storage client could send without
// going through the query service's own shape validation (RFC 0005 §7) is refused here too,
// rather than silently under-matching or panicking on an out-of-range argument index.
func TestValidateFilterShape_Invalid(t *testing.T) {
	nameRef := fieldRef(expression.LevelSpan, expression.SpanFieldName)
	tests := []struct {
		name       string
		filter     *expression.Call
		wantErrIs  error
		wantErrMsg string
	}{
		{
			name:      "unsupported operator",
			filter:    call("bogus"),
			wantErrIs: tracestore.ErrFilterUnsupported,
		},
		{
			name:      "and with no args",
			filter:    call(expression.OpAnd),
			wantErrIs: tracestore.ErrFilterInvalid,
		},
		{
			name:       "and given a value instead of a predicate",
			filter:     call(expression.OpAnd, str("x")),
			wantErrIs:  tracestore.ErrFilterInvalid,
			wantErrMsg: "combines predicates, not values",
		},
		{
			name:      "not with two args",
			filter:    call(expression.OpNot, call(expression.OpExists, nameRef), call(expression.OpExists, nameRef)),
			wantErrIs: tracestore.ErrFilterInvalid,
		},
		{
			name:       "not given a value instead of a predicate",
			filter:     call(expression.OpNot, str("x")),
			wantErrIs:  tracestore.ErrFilterInvalid,
			wantErrMsg: "negates a predicate, not a value",
		},
		{
			name:      "exists with two args",
			filter:    call(expression.OpExists, nameRef, nameRef),
			wantErrIs: tracestore.ErrFilterInvalid,
		},
		{
			name:      "eq with one arg",
			filter:    call(expression.OpEq, nameRef),
			wantErrIs: tracestore.ErrFilterInvalid,
		},
		{
			name:      "eq with three args",
			filter:    call(expression.OpEq, nameRef, str("x"), str("y")),
			wantErrIs: tracestore.ErrFilterInvalid,
		},
		{
			name:      "in with one arg",
			filter:    call(expression.OpIn, nameRef),
			wantErrIs: tracestore.ErrFilterInvalid,
		},
		{
			name:      "some with one arg",
			filter:    call(expression.OpSome, &expression.NestedRef{Level: expression.LevelEvent}),
			wantErrIs: tracestore.ErrFilterInvalid,
		},
		{
			name:       "some with a non-collection first arg",
			filter:     call(expression.OpSome, nameRef, call(expression.OpExists, nameRef)),
			wantErrIs:  tracestore.ErrFilterInvalid,
			wantErrMsg: "quantifies over an event or link collection",
		},
		{
			name:       "some quantifying a value instead of a predicate",
			filter:     call(expression.OpSome, &expression.NestedRef{Level: expression.LevelEvent}, str("x")),
			wantErrIs:  tracestore.ErrFilterInvalid,
			wantErrMsg: "quantifies a predicate, not a value",
		},
		{
			name:       "invalid shape nested under a valid combinator",
			filter:     call(expression.OpAnd, call(expression.OpExists, nameRef), call(expression.OpNot)),
			wantErrIs:  tracestore.ErrFilterInvalid,
			wantErrMsg: `"not" cannot take 0 arguments`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilterShape(tt.filter)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.wantErrIs)
			if tt.wantErrMsg != "" {
				assert.ErrorContains(t, err, tt.wantErrMsg)
			}
		})
	}
}
