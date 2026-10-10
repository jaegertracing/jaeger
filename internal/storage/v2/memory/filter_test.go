// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// p builds the filters under test. The builder holds no state, so one value serves every test.
// The call, fieldRef and attrRef helpers below remain where the hand-built tree is the point: a
// malformed shape, a constant on the left of a comparison, or a reference read back by level
// and name.
var p builder.Predicate

// newFilterFixture builds a span, in the context of a resource and scope,
// with enough shape (attributes at every level, an event and a link) for the
// filter tests below to exercise every level and operator without each test
// constructing its own trace from scratch.
type filterFixture struct {
	t                 *testing.T
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

	return filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: span}
}

func (f filterFixture) matches(filter *expression.Call) bool {
	prepared, err := prepareFilter(filter)
	require.NoError(f.t, err)
	return matchesFilter(prepared, f.resource, f.scope, f.span, f.resourceSchemaURL, f.scopeSchemaURL)
}

// evaluates runs the evaluator on a filter prepareFilter would refuse, for the tests of the
// evaluator's own defenses against a malformed tree.
func (f filterFixture) evaluates(filter *expression.Call) bool {
	return evalPredicate(filter, filterCtx{
		resource: f.resource, scope: f.scope, span: f.span,
		resourceSchemaURL: f.resourceSchemaURL, scopeSchemaURL: f.scopeSchemaURL,
	})
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

func TestMatchesFilter_NilFilterMatchesEverything(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(nil))
}

func TestMatchesFilter_And(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.And(
		p.Span().Name.Eq("POST /cart"),
		p.Span().Status.Eq("error"),
	)))
	assert.False(t, f.matches(p.And(
		p.Span().Name.Eq("POST /cart"),
		p.Span().Status.Eq("ok"),
	)))
}

func TestMatchesFilter_Or(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.Or(
		p.Span().Status.Eq("ok"),
		p.Span().Status.Eq("error"),
	)))
	assert.False(t, f.matches(p.Or(
		p.Span().Status.Eq("ok"),
		p.Span().Status.Eq("unset"),
	)))
}

func TestMatchesFilter_Not(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.Not(p.Span().Status.Eq("ok"))))
	assert.False(t, f.matches(p.Not(p.Span().Status.Eq("error"))))
}

func TestMatchesFilter_SpanFields(t *testing.T) {
	f := newFilterFixture(t)
	tests := []struct {
		name string
		pred *expression.Call
		want bool
	}{
		{"traceID eq", p.Span().TraceID.Eq(f.span.TraceID().String()), true},
		{"spanID eq", p.Span().SpanID.Eq(f.span.SpanID().String()), true},
		{"parentSpanID absent", p.Span().ParentSpanID.Exists(), false},
		{"name eq", p.Span().Name.Eq("POST /cart"), true},
		{"name ne no match", p.Span().Name.Ne("POST /cart"), false},
		{"name ne match", p.Span().Name.Ne("GET /cart"), true},
		{"kind eq", p.Span().Kind.Eq("server"), true},
		{"status eq", p.Span().Status.Eq("error"), true},
		{"statusMessage eq", p.Span().StatusMessage.Eq("boom"), true},
		{"duration gt", p.Span().Duration.Gt(100 * time.Millisecond), true},
		{"duration lt", p.Span().Duration.Lt(100 * time.Millisecond), false},
		{"duration gt, typed", p.Span().Duration.Gt(p.Duration(100 * time.Millisecond)), true},
		{"startTime exists", p.Span().StartTime.Exists(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, f.matches(tt.pred))
		})
	}
}

func TestMatchesFilter_ResourceAndScopeFields(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.Resource().Service.Eq("checkout")))
	assert.False(t, f.matches(p.Resource().Service.Eq("other")))
	assert.True(t, f.matches(p.Scope().Name.Eq("otelgrpc")))
	assert.True(t, f.matches(p.Scope().Version.Eq("1.2.3")))
}

func TestMatchesFilter_Attributes(t *testing.T) {
	f := newFilterFixture(t)
	// The span attribute rows ask for the typed constant, so each match is the typed one and
	// not a coercion.
	assert.True(t, f.matches(p.Span().Attr("http.method").Eq(p.String("POST"))))
	assert.True(t, f.matches(p.Span().Attr("http.status_code").Eq(p.Int(500))))
	assert.True(t, f.matches(p.Span().Attr("duration_ms").Eq(p.Double(150.5))))
	assert.True(t, f.matches(p.Span().Attr("retry").Eq(p.Bool(false))))
	assert.True(t, f.matches(p.Resource().Attr("deployment.environment").Eq("prod")))
	assert.True(t, f.matches(p.Scope().Attr("scope.tag").Eq("scope-value")))
	assert.False(t, f.matches(p.Span().Attr("nonexistent").Exists()))
}

func TestMatchesFilter_UnqualifiedAttributeSearchesSpanAndResource(t *testing.T) {
	f := newFilterFixture(t)
	// http.method lives only on the span; deployment.environment only on the resource.
	// An unqualified (empty-level) attribute reference must find both.
	assert.True(t, f.matches(p.Attr("http.method").Eq("POST")))
	assert.True(t, f.matches(p.Attr("deployment.environment").Eq("prod")))
	assert.False(t, f.matches(p.Attr("nope").Exists()))
}

func TestMatchesFilter_Regex(t *testing.T) {
	f := newFilterFixture(t)
	// A typed pattern, which the evaluator compiles through its own branch; Matches builds an
	// untyped one, covered by TestMatchesFilter_RegexAcceptsAnUntypedPattern.
	assert.True(t, f.matches(p.Compare(expression.OpRegex, p.Span().Name, p.String("cart$"))))
	assert.False(t, f.matches(p.Compare(expression.OpRegex, p.Span().Name, p.String("^cart"))))
}

func TestMatchesFilter_InAndNotIn(t *testing.T) {
	f := newFilterFixture(t)
	list := p.List(expression.ValueTypeString, "GET /cart", "POST /cart")
	assert.True(t, f.matches(p.Span().Name.In(list)))
	// not_in is false when the value is found in the list, same list that made in true.
	assert.False(t, f.matches(p.Span().Name.NotIn(list)))

	otherList := p.List(expression.ValueTypeString, "GET /cart")
	assert.False(t, f.matches(p.Span().Name.In(otherList)))
	assert.True(t, f.matches(p.Span().Name.NotIn(otherList)))

	// not_in on an absent reference is false, same as every other leaf comparison;
	// only a boolean `not` around it would make an absent reference match.
	assert.False(t, f.matches(p.Span().ParentSpanID.NotIn(otherList)))
}

func TestMatchesFilter_GteLte(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.Span().Duration.Gte(150*time.Millisecond)))
	assert.True(t, f.matches(p.Span().Duration.Gte(100*time.Millisecond)))
	assert.False(t, f.matches(p.Span().Duration.Gte(200*time.Millisecond)))

	assert.True(t, f.matches(p.Span().Duration.Lte(150*time.Millisecond)))
	assert.True(t, f.matches(p.Span().Duration.Lte(200*time.Millisecond)))
	assert.False(t, f.matches(p.Span().Duration.Lte(100*time.Millisecond)))
}

func TestMatchesFilter_AbsentReferenceLeafComparisonsAreFalse(t *testing.T) {
	f := newFilterFixture(t)
	absent := p.Span().ParentSpanID
	assert.False(t, f.matches(absent.Eq("anything")))
	assert.False(t, f.matches(absent.Ne("anything")))
	assert.False(t, f.matches(absent.Gt("anything")))
	assert.False(t, f.matches(absent.Matches(".*")))
	// Only `not` turns the absence into a match.
	assert.True(t, f.matches(p.Not(absent.Eq("anything"))))
}

func TestMatchesFilter_EventUncorrelatedMatchesAnyElement(t *testing.T) {
	f := newFilterFixture(t)
	// Outside `some`, event.name and event fields from *different* events both
	// match independently (uncorrelated "any element" reading, RFC 0005 §5.5) —
	// even though no single event has both this name and this attribute.
	assert.True(t, f.matches(p.And(
		p.Event().Name.Eq("retry-scheduled"),
		p.Event().Attr("exception.type").Eq("TimeoutError"),
	)))
}

func TestMatchesFilter_SomeCorrelatesASingleEvent(t *testing.T) {
	f := newFilterFixture(t)
	// Correlated: some single event named "exception" also carries exception.type.
	correlated := p.Some(p.Event(),
		p.And(
			p.Event().Name.Eq("exception"),
			p.Event().Attr("exception.type").Eq("TimeoutError"),
		),
	)
	assert.True(t, f.matches(correlated))

	// No single event is named "retry-scheduled" and also carries exception.type.
	uncorrelated := p.Some(p.Event(),
		p.And(
			p.Event().Name.Eq("retry-scheduled"),
			p.Event().Attr("exception.type").Eq("TimeoutError"),
		),
	)
	assert.False(t, f.matches(uncorrelated))
}

func TestMatchesFilter_SomeOverLinks(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.Some(p.Link(),
		p.Link().Attr("link.tag").Eq("link-value"),
	)))
	assert.False(t, f.matches(p.Some(p.Link(),
		p.Link().Attr("link.tag").Eq("nope"),
	)))
}

func TestMatchesFilter_TimeSinceStart(t *testing.T) {
	f := newFilterFixture(t)
	// The "exception" event fires 50us after the span starts.
	assert.True(t, f.matches(p.Some(p.Event(),
		p.And(
			p.Event().Name.Eq("exception"),
			p.Event().TimeSinceStart.Gt(40*time.Microsecond),
		),
	)))
}

func TestMatchesFilter_UnknownOperatorDoesNotMatch(t *testing.T) {
	f := newFilterFixture(t)
	assert.False(t, f.evaluates(call(expression.Operator("made_up"), fieldRef(expression.LevelSpan, expression.SpanFieldName))))
}

func TestMatchesFilter_NonCallExpressionDoesNotMatch(t *testing.T) {
	f := newFilterFixture(t)
	assert.False(t, evalPredicate(p.String("not a call"), filterCtx{span: f.span}))
}

func TestMatchesFilter_LinkFields(t *testing.T) {
	f := newFilterFixture(t)
	link := f.span.Links().At(0)
	assert.True(t, f.matches(p.Link().TraceID.Eq(link.TraceID().String())))
	assert.True(t, f.matches(p.Link().SpanID.Eq(link.SpanID().String())))
	assert.False(t, f.matches(p.Link().TraceState.Exists()))
}

func TestMatchesFilter_StatusWords(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()

	okSpan := ss.Spans().AppendEmpty()
	okSpan.Status().SetCode(ptrace.StatusCodeOk)
	okFixture := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: okSpan}
	assert.True(t, okFixture.matches(p.Span().Status.Eq("ok")))

	unsetSpan := ss.Spans().AppendEmpty()
	unsetFixture := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: unsetSpan}
	assert.True(t, unsetFixture.matches(p.Span().Status.Eq("unset")))
}

func TestMatchesFilter_ResourceAndScopeMissingFields(t *testing.T) {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	f := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: span}

	// No service.name resource attribute, no scope name/version, and this
	// fixture's ResourceSpans/ScopeSpans carry no schema URL either.
	assert.False(t, f.matches(p.Resource().Service.Exists()))
	assert.False(t, f.matches(p.Resource().SchemaURL.Exists()))
	assert.False(t, f.matches(p.Scope().Name.Exists()))
	assert.False(t, f.matches(p.Scope().Version.Exists()))
	assert.False(t, f.matches(p.Scope().SchemaURL.Exists()))
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
		t:        t,
		resource: rs.Resource(), scope: ss.Scope(), span: span,
		resourceSchemaURL: rs.SchemaUrl(), scopeSchemaURL: ss.SchemaUrl(),
	}

	assert.True(t, f.matches(p.Resource().SchemaURL.Eq("https://opentelemetry.io/schemas/1.9.0")))
	assert.False(t, f.matches(p.Resource().SchemaURL.Eq("other")))
	assert.True(t, f.matches(p.Scope().SchemaURL.Eq("https://opentelemetry.io/schemas/1.4.0")))
}

func TestMatchesFilter_EventTimeField(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.Some(p.Event(),
		p.And(
			p.Event().Name.Eq("exception"),
			p.Event().Time.Exists(),
		),
	)))
}

func TestMatchesFilter_InAndNotInAgainstNonList(t *testing.T) {
	f := newFilterFixture(t)
	// The builder wraps a bare value into a list, so the tree is written out.
	name := fieldRef(expression.LevelSpan, expression.SpanFieldName)
	notAList := p.String("not-a-list")
	assert.False(t, f.matches(call(expression.OpIn, name, notAList)))
	assert.False(t, f.matches(call(expression.OpNotIn, name, notAList)))
}

func TestMatchesFilter_InNumericAndBoolList(t *testing.T) {
	f := newFilterFixture(t)
	numList := p.List(expression.ValueTypeInt, 500, 404)
	assert.True(t, f.matches(p.Span().Attr("http.status_code").In(numList)))

	boolList := p.List(expression.ValueTypeBool, false)
	assert.True(t, f.matches(p.Span().Attr("retry").In(boolList)))
}

func TestMatchesFilter_RegexInvalidPatternOperand(t *testing.T) {
	// The pattern must be a constant, so that it is compiled once before the scan; a
	// reference in its place is refused when the filter is prepared.
	_, err := prepareFilter(call(expression.OpRegex, fieldRef(expression.LevelSpan, expression.SpanFieldName), fieldRef(expression.LevelSpan, expression.SpanFieldParentSpanID)))
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid)
}

func TestMatchesFilter_SomeOverUnknownLevelDoesNotMatch(t *testing.T) {
	f := newFilterFixture(t)
	// The builder only quantifies over events and links, so the tree is written out.
	assert.False(t, f.matches(call(expression.OpSome, &expression.NestedRef{Level: expression.LevelSpan},
		p.Span().Name.Exists(),
	)))
}

func TestMatchesFilter_SomeWithNonNestedRefCollectionDoesNotMatch(t *testing.T) {
	f := newFilterFixture(t)
	assert.False(t, f.evaluates(call(expression.OpSome, p.String("not-a-collection"),
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
	assert.False(t, f.matches(p.Span().Attr("http.status_code").Eq(p.String("500"))))
	assert.True(t, f.matches(p.Span().Attr("http.status_code").Ne(p.String("500"))),
		"present but a different kind, so ne is true rather than the leaf-absence false")
}

// TestMatchesFilter_UntypedConstantResolvesAgainstAttributeKind pins the other half of the
// same fix: an AnyValue constant (the caller wrote no type) resolves against whichever kind
// the paired attribute actually turns out to hold, rather than being read as a string.
func TestMatchesFilter_UntypedConstantResolvesAgainstAttributeKind(t *testing.T) {
	f := newFilterFixture(t)
	// http.status_code (int 500) and retry (bool false) beside an untyped "500"/"false".
	assert.True(t, f.matches(p.Span().Attr("http.status_code").Eq("500")))
	assert.True(t, f.matches(p.Span().Attr("retry").Eq("false")))
	// An untyped value that cannot be read as the attribute's kind matches nothing.
	assert.False(t, f.matches(p.Span().Attr("http.status_code").Eq("not-a-number")))
}

// TestMatchesFilter_OpaqueAttributeExistsButNeverCompares pins OpExists to presence alone: a
// bytes-valued attribute has no scalar reading, but it is still present, so exists must be
// true while eq/gt/lt, which need a scalar, never match it — and ne, which only needs presence
// and non-equality, is true.
func TestMatchesFilter_OpaqueAttributeExistsButNeverCompares(t *testing.T) {
	f := newFilterFixture(t)
	f.span.Attributes().PutEmptyBytes("payload").FromRaw([]byte{1, 2, 3})

	assert.True(t, f.matches(p.Span().Attr("payload").Exists()))
	assert.False(t, f.matches(p.Span().Attr("payload").Eq(p.String("anything"))))
	assert.True(t, f.matches(p.Span().Attr("payload").Ne(p.String("anything"))))
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
	f := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: span}

	assert.True(t, f.matches(p.Span().StartTime.Eq(p.Int(nanos))))
	assert.False(t, f.matches(p.Span().StartTime.Eq(p.Int(nanos-1))))
}

// TestMatchesFilter_ListTypeIsAuthoritative pins RFC 0005 §5.4: a typed list matches only
// values of its declared kind, so a numeric attribute does not match a string-typed list that
// happens to contain its digits, and vice versa.
func TestMatchesFilter_ListTypeIsAuthoritative(t *testing.T) {
	f := newFilterFixture(t)
	// http.status_code is an int attribute (500).
	stringList := p.List(expression.ValueTypeString, "500")
	assert.False(t, f.matches(p.Span().Attr("http.status_code").In(stringList)))

	// http.method is a string attribute ("POST"), never matches an int-typed list.
	intList := p.List(expression.ValueTypeInt, 500)
	assert.False(t, f.matches(p.Span().Attr("http.method").In(intList)))
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

	f := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: span}
	assert.True(t, f.matches(p.Span().TraceState.Eq("congo=t61rcWkgMzE")))
	assert.True(t, f.matches(p.Span().EndTime.Eq(p.Int(int64(span.EndTimestamp())))))
	assert.True(t, f.matches(p.Some(p.Link(),
		p.Link().TraceState.Eq("congo=t61rcWkgMzE"),
	)))
}

func TestCompareValues_BoolOrdering(t *testing.T) {
	trueVal := boolValue(true)
	falseVal := boolValue(false)
	assert.Equal(t, 0, compareValues(trueVal, trueVal))
	assert.Negative(t, compareValues(falseVal, trueVal))
	assert.Positive(t, compareValues(trueVal, falseVal))
}

func TestEvalValueKind(t *testing.T) {
	assert.True(t, comparableKinds(evalValue{kind: kindString}, evalValue{kind: kindString}))
	assert.True(t, comparableKinds(evalValue{kind: kindBool}, evalValue{kind: kindBool}))
	assert.True(t, comparableKinds(evalValue{kind: kindInt}, evalValue{kind: kindDouble}))
	assert.False(t, comparableKinds(evalValue{kind: kindString}, evalValue{kind: kindBool}))
	assert.False(t, comparableKinds(evalValue{kind: kindOpaque}, evalValue{kind: kindOpaque}))
	assert.False(t, comparableKinds(untypedValue("x"), untypedValue("x")))
}

func TestCoerceUntyped(t *testing.T) {
	tests := []struct {
		name    string
		v       evalValue
		other   evalValue
		wantOK  bool
		wantVal evalValue
	}{
		{"reads as bool", untypedValue("true"), evalValue{kind: kindBool}, true, boolValue(true)},
		{"not a bool", untypedValue("nope"), evalValue{kind: kindBool}, false, evalValue{}},
		{"reads as int", untypedValue("500"), evalValue{kind: kindInt}, true, intValue(500)},
		{"reads as double when not an exact int", untypedValue("1.5"), evalValue{kind: kindDouble}, true, doubleValue(1.5)},
		{"not a number", untypedValue("nope"), evalValue{kind: kindInt}, false, evalValue{}},
		{"reads as string against a string", untypedValue("hi"), evalValue{kind: kindString}, true, stringValue("hi")},
		{"reads as string against another untyped value", untypedValue("hi"), untypedValue("hi"), true, stringValue("hi")},
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
		_, _, ok := resolveComparable(evalValue{kind: kindOpaque}, stringValue("x"))
		assert.False(t, ok)
		_, _, ok = resolveComparable(stringValue("x"), evalValue{kind: kindOpaque})
		assert.False(t, ok)
	})
	t.Run("untyped left resolves against typed right", func(t *testing.T) {
		a, b, ok := resolveComparable(untypedValue("500"), intValue(500))
		require.True(t, ok)
		assert.Equal(t, intValue(500), a)
		assert.Equal(t, intValue(500), b)
	})
	t.Run("untyped right resolves against typed left", func(t *testing.T) {
		a, b, ok := resolveComparable(boolValue(true), untypedValue("true"))
		require.True(t, ok)
		assert.Equal(t, boolValue(true), a)
		assert.Equal(t, boolValue(true), b)
	})
	t.Run("untyped left fails to coerce", func(t *testing.T) {
		_, _, ok := resolveComparable(untypedValue("nope"), evalValue{kind: kindBool})
		assert.False(t, ok)
	})
	t.Run("untyped right fails to coerce", func(t *testing.T) {
		_, _, ok := resolveComparable(evalValue{kind: kindInt}, untypedValue("nope"))
		assert.False(t, ok)
	})
	t.Run("mismatched kinds are never comparable", func(t *testing.T) {
		_, _, ok := resolveComparable(stringValue("x"), evalValue{kind: kindBool})
		assert.False(t, ok)
	})
}

func TestValueInList_EmptyTypeInfersFromOperand(t *testing.T) {
	assert.True(t, valueInList(intValue(500), &expression.List{Values: []string{"500"}}))
	assert.True(t, valueInList(doubleValue(1.5), &expression.List{Values: []string{"1.5"}}))
	assert.True(t, valueInList(boolValue(true), &expression.List{Values: []string{"true"}}))
	assert.False(t, valueInList(evalValue{kind: kindOpaque}, &expression.List{Values: []string{"anything"}}),
		"opaque has no kind to infer, so it never matches an untyped list")
	assert.True(t, valueInList(intValue(500), &expression.List{Values: []string{"500.0"}}),
		"an untyped list matches whatever an untyped eq matches, and eq reads 500.0 against an int attribute as a number")
	assert.False(t, valueInList(intValue(500), &expression.List{Values: []string{"five hundred"}}))
}

func TestValueInList_ExplicitDoubleType(t *testing.T) {
	list := &expression.List{Values: []string{"1.5", "2.5"}, Type: expression.ValueTypeDouble}
	assert.True(t, valueInList(doubleValue(1.5), list))
	assert.False(t, valueInList(doubleValue(3.5), list))
	assert.False(t, valueInList(intValue(1), list), "a typed double list does not fall back to int")
}

func TestAttrToEvalValue_UnsupportedTypeIsOpaque(t *testing.T) {
	m := pcommon.NewMap()
	m.PutEmptyBytes("payload").FromRaw([]byte{1, 2, 3})
	v, ok := m.Get("payload")
	require.True(t, ok)
	resolved := attrToEvalValue(v)
	assert.Equal(t, kindOpaque, resolved.kind, "a bytes-valued attribute has no scalar reading, but is still present")
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
	errorFixture := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: errorSpan}

	okSpan := ss.Spans().AppendEmpty()
	okSpan.Status().SetCode(ptrace.StatusCodeOk)
	okFixture := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: okSpan}

	unsetSpan := ss.Spans().AppendEmpty()
	unsetFixture := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: unsetSpan}

	for _, level := range []expression.Level{"", expression.LevelSpan} {
		assert.True(t, errorFixture.matches(call(expression.OpEq, attrRef(level, errorAttribute), p.Bool(true))),
			"level=%q: an Error-status span matches error=true", level)
		assert.False(t, errorFixture.matches(call(expression.OpEq, attrRef(level, errorAttribute), p.Bool(false))),
			"level=%q: an Error-status span does not match error=false", level)
		// error=false is the complement of error=true, not "tag absent": Unset is by far the
		// common case and must match it, the same way the legacy search treats it.
		assert.True(t, okFixture.matches(call(expression.OpEq, attrRef(level, errorAttribute), p.Bool(false))),
			"level=%q: an Ok-status span matches error=false", level)
		assert.True(t, unsetFixture.matches(call(expression.OpEq, attrRef(level, errorAttribute), p.Bool(false))),
			"level=%q: an Unset-status span matches error=false, not just Ok", level)
		assert.True(t, errorFixture.matches(call(expression.OpExists, attrRef(level, errorAttribute))),
			"level=%q: error always exists, whether or not the span carries the attribute literally", level)
		assert.True(t, okFixture.matches(call(expression.OpNe, attrRef(level, errorAttribute), p.Bool(true))),
			"level=%q: ne is the leaf-present complement, same as any other attribute", level)
	}

	// A literal "error" attribute set on the span is shadowed by the virtual one: the span's
	// actual status still decides, not whatever the attribute map happens to hold.
	shadowed := ss.Spans().AppendEmpty()
	shadowed.Status().SetCode(ptrace.StatusCodeOk)
	shadowed.Attributes().PutBool(errorAttribute, true)
	shadowedFixture := filterFixture{t: t, resource: rs.Resource(), scope: ss.Scope(), span: shadowed}
	assert.True(t, shadowedFixture.matches(call(expression.OpEq, attrRef("", errorAttribute), p.Bool(false))),
		"the literal attribute value is shadowed by the status-derived one")

	// At any other level, "error" is an ordinary attribute lookup, not the virtual field: the
	// special case is specific to the span's own status.
	resourceErrorFixture := newFilterFixture(t)
	resourceErrorFixture.resource.Attributes().PutBool(errorAttribute, true)
	assert.True(t, resourceErrorFixture.matches(call(expression.OpEq, attrRef(expression.LevelResource, errorAttribute), p.Bool(true))),
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
			call(expression.OpExists, nameRef), call(expression.OpEq, nameRef, p.String("x")))},
		{"not", call(expression.OpNot, call(expression.OpExists, nameRef))},
		{"exists", call(expression.OpExists, nameRef)},
		{"eq", call(expression.OpEq, nameRef, p.String("x"))},
		{"regex", call(expression.OpRegex, nameRef, p.String("^x$"))},
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
			filter:     call(expression.OpAnd, p.String("x")),
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
			filter:     call(expression.OpNot, p.String("x")),
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
			filter:    call(expression.OpEq, nameRef, p.String("x"), p.String("y")),
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
			filter:     call(expression.OpSome, &expression.NestedRef{Level: expression.LevelEvent}, p.String("x")),
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

func TestMatchesFilter_RegexAcceptsAnUntypedPattern(t *testing.T) {
	f := newFilterFixture(t)
	nameRef := p.Span().Name
	assert.True(t, f.matches(nameRef.Matches("cart$")))
	assert.False(t, f.matches(nameRef.Matches("^cart")))
}

// TestPrepareFilter_Regex pins that a regex pattern is compiled once, when the filter is
// prepared, and that a pattern the search could not compile ahead of the scan is refused there.
func TestPrepareFilter_Regex(t *testing.T) {
	nameRef := p.Span().Name
	// The builder collapses a one-argument And, so each tree has a second predicate to keep the
	// pattern nested.
	nested := p.And(nameRef.Exists(), p.Not(nameRef.Matches("^x")))
	prepared, err := prepareFilter(nested)
	require.NoError(t, err)
	assert.Len(t, prepared.regexps, 1, "the pattern under and/not is compiled")

	_, err = prepareFilter(nameRef.Matches("("))
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid, "a pattern that does not compile")
	_, err = prepareFilter(p.And(nameRef.Exists(), nameRef.Matches("(")))
	require.ErrorIs(t, err, tracestore.ErrFilterInvalid, "the same pattern nested under and")

	prepared, err = prepareFilter(nil)
	require.NoError(t, err)
	assert.True(t, matchesFilter(prepared, pcommon.NewResource(), pcommon.NewInstrumentationScope(), ptrace.NewSpan(), "", ""),
		"no filter matches every span")
}

// TestMatchesFilter_BooleanHasNoOrder pins RFC 0005 §5.3 for an attribute, whose type the query
// boundary cannot see: a boolean compares for equality but never orders, so gt and its kin
// over one match nothing rather than ranking false below true.
func TestMatchesFilter_BooleanHasNoOrder(t *testing.T) {
	f := newFilterFixture(t)
	f.span.Attributes().PutBool("retry", true)
	retry := p.Span().Attr("retry")
	assert.True(t, f.matches(retry.Eq("true")))
	assert.True(t, f.matches(retry.Ne("false")))
	for _, op := range []expression.Operator{expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte} {
		assert.False(t, f.matches(p.Compare(op, retry, "false")), string(op))
		assert.False(t, f.matches(p.Compare(op, retry, "true")), string(op))
	}
}

// TestMatchesFilter_MembershipOnTimeFields pins that the elements of a list beside a duration or
// timestamp field are read as that type, since they travel as text.
func TestMatchesFilter_MembershipOnTimeFields(t *testing.T) {
	f := newFilterFixture(t)
	duration := p.Span().Duration
	start := p.Span().StartTime
	assert.True(t, f.matches(duration.In(100*time.Millisecond, 150*time.Millisecond)))
	assert.False(t, f.matches(duration.NotIn(100*time.Millisecond, 150*time.Millisecond)))
	assert.True(t, f.matches(start.In(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))))
	assert.False(t, f.matches(duration.In(time.Second)))
	assert.True(t, f.matches(duration.NotIn(time.Second)))
	assert.False(t, f.evaluates(duration.In("not a duration")))
	assert.False(t, f.matches(p.Span().Field("no.such.field").In(time.Second)), "an unknown field resolves to nothing")
}

func TestMatchesFilter_UnspecifiedSpanKind(t *testing.T) {
	f := newFilterFixture(t)
	f.span.SetKind(ptrace.SpanKindUnspecified)
	kind := p.Span().Kind
	assert.True(t, f.matches(kind.Eq("unspecified")))
	assert.True(t, f.matches(kind.In("unspecified", "server")))
	assert.False(t, f.evaluates(kind.Eq("")))
}

func TestMatchesFilter_NaNNeverCompares(t *testing.T) {
	f := newFilterFixture(t)
	ms := p.Span().Attr("duration_ms")
	for _, op := range []expression.Operator{expression.OpEq, expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte} {
		assert.False(t, f.matches(p.Compare(op, ms, "NaN")), string(op))
		assert.False(t, f.matches(p.Compare(op, ms, p.Double(math.NaN()))), string(op))
	}
	assert.True(t, f.matches(ms.Ne(p.Double(math.NaN()))), "present and not equal, as for any incomparable pair")
}

// TestMatchesFilter_AbsentAndUnknownFieldsResolveToNothing pins that a field the span does not
// carry, and a name or level the vocabulary does not define, resolve to no value, so exists is
// false and a comparison never matches.
func TestMatchesFilter_AbsentAndUnknownFieldsResolveToNothing(t *testing.T) {
	f := newFilterFixture(t)
	f.span.Status().SetMessage("")
	for _, ref := range []*expression.FieldRef{
		fieldRef(expression.LevelSpan, expression.SpanFieldTraceState),
		fieldRef(expression.LevelSpan, expression.SpanFieldStatusMessage),
		fieldRef(expression.LevelSpan, expression.SpanFieldParentSpanID),
		fieldRef(expression.LevelResource, "no.such.field"),
		fieldRef(expression.LevelScope, "no.such.field"),
		fieldRef(expression.LevelEvent, "no.such.field"),
		fieldRef(expression.LevelLink, "no.such.field"),
		fieldRef("no.such.level", expression.SpanFieldName),
	} {
		assert.False(t, f.matches(call(expression.OpExists, ref)), "%s.%s", ref.Level, ref.Name)
	}

	f.span.SetParentSpanID(pcommon.SpanID{9})
	assert.True(t, f.matches(p.Span().ParentSpanID.Eq(pcommon.SpanID{9}.String())))
}

func TestMatchesFilter_LinkAttributeOutsideSome(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.Link().Attr("link.tag").Eq("link-value")))
	assert.False(t, f.matches(p.Link().Attr("no.such.attr").Exists()))
}

func TestMatchesFilter_TypedListMismatches(t *testing.T) {
	f := newFilterFixture(t)
	status := p.Span().Attr("http.status_code") // int 500
	method := p.Span().Attr("http.method")      // string
	retry := p.Span().Attr("retry")             // bool false
	assert.False(t, f.matches(status.In(p.List(expression.ValueTypeInt, 404))))
	assert.False(t, f.matches(method.In(p.List(expression.ValueTypeBool, true))))
	assert.False(t, f.matches(retry.In(p.List(expression.ValueTypeBool, true))))
	assert.False(t, f.matches(status.In(p.List("no-such-type", 500))))
}

// TestMatchesFilter_LegacySyntheticTags pins that the attribute names the legacy predicate
// fields use for span kind, span status and the scope's name and version read the span's own
// fields, as validSpan reads them, so a legacy query an interceptor turned into a filter still
// matches on a backend that declares filter support.
func TestMatchesFilter_LegacySyntheticTags(t *testing.T) {
	f := newFilterFixture(t)
	f.span.SetKind(ptrace.SpanKindServer)
	f.span.Status().SetCode(ptrace.StatusCodeError)
	for _, level := range []expression.Level{"", expression.LevelSpan} {
		assert.True(t, f.matches(call(expression.OpEq, attrRef(level, "span.kind"), &expression.AnyValue{Value: "server"})))
		assert.False(t, f.matches(call(expression.OpEq, attrRef(level, "span.kind"), &expression.AnyValue{Value: "client"})))
		assert.True(t, f.matches(call(expression.OpEq, attrRef(level, "span.status"), &expression.AnyValue{Value: "error"})))
		assert.True(t, f.matches(call(expression.OpEq, attrRef(level, "scope.name"), &expression.AnyValue{Value: "otelgrpc"})))
		assert.True(t, f.matches(call(expression.OpEq, attrRef(level, "scope.version"), &expression.AnyValue{Value: "1.2.3"})))
	}
	assert.False(t, f.matches(call(expression.OpExists, attrRef(expression.LevelResource, "span.kind"))),
		"only a span-level or unqualified reference is the legacy spelling")
}

// TestMatchesFilter_LegacyResourcePrefix pins that an unqualified attribute reference whose key
// starts with "resource." reads the resource's attribute, as the legacy predicate fields do, so
// a legacy query an interceptor turned into a filter still matches on a backend that declares
// filter support. A span-level reference is not the legacy spelling and reads the literal key.
func TestMatchesFilter_LegacyResourcePrefix(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(p.Attr("resource.deployment.environment").Eq("prod")))
	assert.False(t, f.matches(p.Attr("resource.deployment.environment").Eq("staging")))
	assert.False(t, f.matches(p.Span().Attr("resource.deployment.environment").Exists()))
	assert.False(t, f.matches(p.Attr("resource.http.status_code").Exists()), "a span attribute is not behind the prefix")
}

// TestMatchesFilter_UntypedConstantBesideTypedConstant pins that an untyped constant beside a
// typed one takes the typed one's numeric type, so a comparison of two constants that the query
// boundary accepts (RFC 0005 §5.4) has its arithmetic meaning.
func TestMatchesFilter_UntypedConstantBesideTypedConstant(t *testing.T) {
	f := newFilterFixture(t)
	assert.True(t, f.matches(call(expression.OpEq, &expression.AnyValue{Value: "1"}, p.Double(1))))
	assert.True(t, f.matches(call(expression.OpEq, &expression.AnyValue{Value: "1"}, p.Int(1))))
	assert.True(t, f.matches(call(expression.OpLt, &expression.AnyValue{Value: "0.5"}, p.Double(1))))
	assert.False(t, f.matches(call(expression.OpLt, &expression.AnyValue{Value: "0.5"}, p.Int(1))),
		"a typed int is authoritative, and 0.5 is not an int")
}

// TestMatchesFilter_MixedNumericKinds pins RFC 0005 §5.4 for numbers: an untyped constant
// compares with an attribute of either numeric type, while a typed int or double constant
// matches only an attribute stored as that type, as a typed list already does.
func TestMatchesFilter_MixedNumericKinds(t *testing.T) {
	f := newFilterFixture(t)
	durationMs := p.Span().Attr("duration_ms")  // double 150.5
	status := p.Span().Attr("http.status_code") // int 500
	assert.True(t, f.matches(durationMs.Gt("100")), "double 150.5 > untyped 100")
	assert.True(t, f.matches(status.Lt("600.5")), "int 500 < untyped 600.5")
	assert.True(t, f.matches(status.Eq("500.0")), "int 500 == untyped 500.0")

	assert.False(t, f.matches(durationMs.Gt(p.Int(100))), "a typed int matches no double attribute")
	assert.False(t, f.matches(status.Lt(p.Double(600.5))), "a typed double matches no int attribute")
	assert.False(t, f.matches(status.Eq(p.Double(500))), "not even one holding the same number")
	assert.True(t, f.matches(status.Eq(p.Int(500))))
	assert.True(t, f.matches(durationMs.Eq(p.Double(150.5))))
}

// TestCompareValues_IntKeepsPrecisionAgainstIntegralDouble pins that an int64 above 2^53 is not
// rounded to float64 when the double it compares with is itself an integer.
func TestCompareValues_IntKeepsPrecisionAgainstIntegralDouble(t *testing.T) {
	big := intValue(1<<53 + 1)
	rounded := doubleValue(1 << 53)
	assert.Positive(t, compareValues(big, rounded))
	assert.Negative(t, compareValues(rounded, big))
	assert.Equal(t, 0, compareValues(intValue(1<<53), rounded))
	assert.Negative(t, compareValues(intValue(1), doubleValue(1.5)), "a fractional double compares in floating point")
	assert.Positive(t, compareValues(doubleValue(1.5), intValue(1)))
	assert.Negative(t, compareValues(intValue(math.MaxInt64), doubleValue(1e19)), "a double beyond int64 is beyond every integer")
	assert.Negative(t, compareValues(intValue(math.MaxInt64), doubleValue(1<<63)),
		"2^63 rounds to the same float64 as MaxInt64, but is beyond it")
	assert.Positive(t, compareValues(doubleValue(1<<63), intValue(math.MaxInt64)))
	assert.Positive(t, compareValues(intValue(math.MinInt64), doubleValue(-1e19)))
}
