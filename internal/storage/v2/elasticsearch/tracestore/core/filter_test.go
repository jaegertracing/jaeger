// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	builder "github.com/jaegertracing/jaeger/internal/expression"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// The cases below are written with the Predicate builder wherever it can express them. These three
// helpers serve the handful that it cannot: an operator holding the wrong number or kind of
// arguments. The builder gives each operator its own method, so it has no way to say "eq with one
// operand", and it keeps the reference term private, so such a tree cannot be assembled from what a
// chain returns either — which is the builder working as intended, and why these cases go to the
// AST directly. A remote-storage client can still put one on the wire, so the reader is tested
// against them.
func spanAttr(name string) *expression.AttributeRef {
	return &expression.AttributeRef{Key: name, Level: expression.LevelSpan}
}

func scalar(value string) *expression.AnyValue {
	return &expression.AnyValue{Value: value}
}

func call(op expression.Operator, args ...expression.Expression) *expression.Call {
	return &expression.Call{Op: op, Args: args}
}

// TestFilterCapabilities pins what this reader declares it can serve, because the query
// service refuses a filter on the strength of that declaration alone: a level or operator
// missing here is one no caller can reach, and one listed here that buildFilterQuery cannot
// lower would reach the reader and fail late.
func TestFilterCapabilities(t *testing.T) {
	caps := FilterCapabilities()
	assert.Equal(t, []expression.Level{
		expression.LevelSpan,
		expression.LevelResource,
		expression.LevelScope,
		expression.LevelEvent,
		expression.LevelLink,
	}, caps.Levels)
	for _, op := range []expression.Operator{
		expression.OpAnd, expression.OpOr, expression.OpNot,
		expression.OpEq, expression.OpNe, expression.OpGt, expression.OpLt,
		expression.OpGte, expression.OpLte, expression.OpRegex, expression.OpExists,
		expression.OpIn, expression.OpNotIn,
	} {
		assert.True(t, caps.SupportsOperator(op), "expected %q to be declared", op)
	}
	assert.False(t, caps.SupportsOperator(expression.OpSome),
		"correlated matching over a span's events is not implemented")
	assert.True(t, caps.SupportsLevel(expression.LevelScope))
	assert.True(t, caps.SupportsLevel(expression.LevelLink))
}

// filterSnapshots is where the lowered form of each accepted case is committed, one file per case
// named after it, so a case that is renamed or deleted shows up as a snapshot nothing claims.
const filterSnapshots = "testdata/filter"

// TestBuildFilterQuery covers what this schema can answer, one predicate at a time. The queries are
// written with the Predicate builder a caller uses rather than as AST literals, so a case reads as
// the query it stands for; the few cases that need a tree the builder does not produce say so.
//
// The lowered query is compared against a committed snapshot, which is the convention in this
// package: the expected Elasticsearch JSON is large enough that inlining it hides the case.
func TestBuildFilterQuery(t *testing.T) {
	var p builder.Predicate
	tests := []struct {
		name   string
		filter *expression.Call
	}{
		{
			name:   "unqualified attribute searches the span and resource levels",
			filter: p.Attr("http.status_code").Eq("500"),
		},
		{
			name:   "span level searches only the span's own attributes",
			filter: p.Span().Attr("component").Eq("grpc"),
		},
		{
			name:   "resource level searches only the process attributes",
			filter: p.Resource().Attr("deployment.environment").Eq("staging"),
		},
		{
			name:   "event level has one location, so it needs no disjunction",
			filter: p.Event().Attr("exception.type").Eq("IOError"),
		},
		{
			name:   "event.name reads the logs.fields entry the write path stores it in",
			filter: p.Event().Name.Eq("exception"),
		},
		{
			name:   "span.name is the operation name",
			filter: p.Span().Name.Eq("/api/v3/traces"),
		},
		{
			name:   "resource.service is the service name",
			filter: p.Resource().Service.Eq("cart"),
		},
		{
			name:   "span.traceID is a keyword of the span document",
			filter: p.Span().TraceID.Eq("00000000000000000000000000000f01"),
		},
		{
			name:   "span.spanID is the keyword beside it",
			filter: p.Span().SpanID.Eq("0000000000000f01"),
		},
		{
			name:   "a pattern on span.traceID, which is text like any other keyword",
			filter: p.Span().TraceID.Matches("0f0[12]"),
		},
		{
			name:   "scope.name reads the span tag the write path folds the scope into",
			filter: p.Scope().Name.Eq("io.opentelemetry.contrib.cart"),
		},
		{
			name:   "scope.version reads the tag beside it",
			filter: p.Scope().Version.Eq("1.2.0"),
		},
		{
			name:   "link.traceID enters the references nested type the write path stores a link in",
			filter: p.Link().TraceID.Eq("00000000000000000000000000000f02"),
		},
		{
			name:   "link.spanID is the keyword beside it, in the same nested document",
			filter: p.Link().SpanID.Eq("0000000000000f02"),
		},
		{
			name:   "span.duration compares microseconds against a value carrying its unit",
			filter: p.Span().Duration.Gt("2s"),
		},
		{
			name:   "gte on the duration",
			filter: p.Span().Duration.Gte("1500ms"),
		},
		{
			name:   "lt on the duration",
			filter: p.Span().Duration.Lt("1m"),
		},
		{
			name:   "lte on the duration",
			filter: p.Span().Duration.Lte("500us"),
		},
		{
			name:   "eq on the duration",
			filter: p.Span().Duration.Eq("3s"),
		},
		{
			name:   "span.startTime compares microseconds against an instant",
			filter: p.Span().StartTime.Gte("2026-08-16T18:56:20.123456789Z"),
		},
		{
			name:   "lt on the start time",
			filter: p.Span().StartTime.Lt("2026-08-16T18:56:21Z"),
		},
		{
			name:   "eq on the start time, which is exact to the microsecond the write path kept",
			filter: p.Span().StartTime.Eq("2026-08-16T18:56:20.123456Z"),
		},
		{
			name:   "an instant before the epoch, which the column holds as a negative number",
			filter: p.Span().StartTime.Gt("1969-12-31T23:59:59Z"),
		},
		{
			name: "a timestamp constant, which is what a finalized filter carries",
			filter: p.Span().StartTime.Gt(&expression.TimestampValue{
				Value: time.Date(2026, time.August, 16, 18, 56, 20, 123456789, time.UTC),
			}),
		},
		{
			name:   "event.time enters the nested logs document the write path stores an event in",
			filter: p.Event().Time.Lte("2026-08-16T18:56:20Z"),
		},
		{
			name:   "a string constant against the operation name, which is what finalizing produces",
			filter: p.Span().Name.Eq(p.Text("checkout")),
		},
		{
			name:   "a string constant against the service name",
			filter: p.Resource().Service.Eq(p.Text("cart")),
		},
		{
			name:   "a string constant against the event name",
			filter: p.Event().Name.Eq(p.Text("exception")),
		},
		{
			// A Go duration reaches the AST as the untyped constant "2s", so the typed constant
			// resolving produces is written out here.
			name:   "a duration constant, which is what a finalized filter carries",
			filter: p.Span().Duration.Gt(&expression.DurationValue{Value: 2 * time.Second}),
		},
		{
			name:   "regex matches anywhere in the value, which this engine needs told",
			filter: p.Span().Name.Matches("GET .*"),
		},
		{
			name:   "regex on an attribute reaches every location the attribute lives in",
			filter: p.Span().Attr("http.route").Matches("/api/.*"),
		},
		{
			name:   "an escaped punctuation character, which both dialects read the same way",
			filter: p.Span().Attr("http.route").Matches(`/cart\.json`),
		},
		{
			name:   "an escaped backslash, which does not escape what follows it",
			filter: p.Span().Attr("http.route").Matches(`a\\d`),
		},
		{
			name:   "an alternation is grouped, so the wildcards apply to the whole pattern",
			filter: p.Span().Attr("http.route").Matches("cart|checkout"),
		},
		{
			name:   "exists tests the key alone",
			filter: p.Event().Attr("exception.stacktrace").Exists(),
		},
		{
			name:   "exists on an attribute tests the key in every location it may live in",
			filter: p.Span().Attr("http.route").Exists(),
		},
		{
			name:   "exists on event.name tests the key the write path stores it under",
			filter: p.Event().Name.Exists(),
		},
		{
			name:   "exists on a built-in field tests its own field",
			filter: p.Span().Duration.Exists(),
		},
		{
			name:   "exists on the operation name",
			filter: p.Span().Name.Exists(),
		},
		{
			name:   "exists on the service name",
			filter: p.Resource().Service.Exists(),
		},
		{
			name:   "exists on span.traceID",
			filter: p.Span().TraceID.Exists(),
		},
		{
			name:   "exists on scope.name tests the tag the write path folds it into",
			filter: p.Scope().Name.Exists(),
		},
		{
			name:   "exists on link.spanID enters the nested type before testing the field",
			filter: p.Link().SpanID.Exists(),
		},
		{
			name:   "exists on span.startTime",
			filter: p.Span().StartTime.Exists(),
		},
		{
			name:   "exists on event.time",
			filter: p.Event().Time.Exists(),
		},
		{
			name:   "and becomes the must clause",
			filter: p.And(p.Resource().Service.Eq("cart"), p.Span().Duration.Gt("2s")),
		},
		{
			// A one-argument conjunction is not a filter a caller can send through the query
			// service, which validates arity first, and the builder collapses it to the predicate
			// it wraps, so it is written out here. It can still arrive from a remote-storage
			// client, and it means that predicate.
			name: "a conjunction of one predicate is that predicate",
			filter: &expression.Call{
				Op:   expression.OpAnd,
				Args: []expression.Expression{p.Resource().Service.Eq("cart")},
			},
		},
		{
			name:   "or becomes the should clause",
			filter: p.Or(p.Resource().Service.Eq("cart"), p.Resource().Service.Eq("checkout")),
		},
		{
			name:   "not becomes the must_not clause, which also matches a span missing the reference",
			filter: p.Not(p.Resource().Service.Eq("healthcheck")),
		},
		{
			name: "nesting composes to nested bool queries",
			filter: p.And(
				p.Resource().Service.Eq("cart"),
				p.Or(
					p.Span().Name.Eq("a"),
					p.Not(p.Span().Name.Eq("b")),
				),
			),
		},
		{
			name:   "ne requires the reference to be present, so an absent one does not match",
			filter: p.Event().Attr("exception.type").Ne("IOError"),
		},
		{
			name:   "ne on a built-in field guards on the field's own presence",
			filter: p.Resource().Service.Ne("cart"),
		},
		{
			name:   "in is the disjunction of equalities it stands for",
			filter: p.Resource().Service.In("cart", "checkout"),
		},
		{
			name:   "a single-member list needs no disjunction",
			filter: p.Resource().Service.In("cart"),
		},
		{
			name:   "not_in requires the reference to be present, like ne",
			filter: p.Resource().Service.NotIn("cart", "checkout"),
		},
		{
			name: "membership on span.traceID, whose members are read as the string the field holds",
			filter: p.Span().TraceID.In(
				"00000000000000000000000000000f01",
				"00000000000000000000000000000f02",
			),
		},
		{
			name: "membership on span.startTime reads each member as an instant",
			filter: p.Span().StartTime.In(
				"2026-08-16T18:56:20Z",
				"2026-08-16T18:56:21Z",
			),
		},
		{
			name:   "ne on link.spanID guards on the nested field's own presence",
			filter: p.Link().SpanID.Ne("0000000000000f02"),
		},
		{
			name:   "ne on scope.name guards on the tag holding it",
			filter: p.Scope().Name.Ne("io.opentelemetry.contrib.cart"),
		},
		{
			// The lowering is the disjunction of equalities the membership stands for, and each
			// element is compared as text, because this schema writes every attribute value as a
			// keyword. A list with no type and a list declaring string therefore ask for the same
			// comparison (the next case); a numeric or boolean element type is refused.
			name:   "membership against an attribute is the disjunction of equalities it stands for",
			filter: p.Span().Attr("http.method").In("GET", "POST"),
		},
		{
			name:   "a list declaring string, beside an attribute this schema matches as text",
			filter: p.Span().Attr("http.method").In(p.List(expression.ValueTypeString, "GET")),
		},
		{
			// The legacy tag search carried a map of strings, so a string constant matched a value
			// of any stored type. A declared string asks for that same comparison, the only one
			// this schema performs on an attribute.
			name:   "a string constant, beside an attribute this schema matches as text",
			filter: p.Span().Attr("http.route").Eq(p.Text("/cart")),
		},
		{
			name:   "not_in against an attribute requires the attribute to be present",
			filter: p.Span().Attr("http.method").NotIn("GET"),
		},
		{
			name:   "membership against an unqualified attribute searches both levels",
			filter: p.Attr("http.method").In("GET"),
		},
		{
			name:   "error=true matches the tag the write path records",
			filter: p.Span().Attr("error").Eq("true"),
		},
		{
			name:   "error=false excludes error=true, because a span that succeeded carries no tag",
			filter: p.Span().Attr("error").Eq("false"),
		},
		{
			name:   "an unqualified error=0 is read the same way as error=false",
			filter: p.Attr("error").Eq("0"),
		},
		{
			name:   "a non-boolean error value keeps its literal match",
			filter: p.Span().Attr("error").Eq("oops"),
		},
		{
			name:   "an error tag at the resource level is an ordinary attribute",
			filter: p.Resource().Attr("error").Eq("false"),
		},
	}
	claimed := make(map[string]bool, len(tests))
	withSpanReader(t, func(r *spanReaderTest) {
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				query, err := r.reader.buildFilterQuery(test.filter)
				require.NoError(t, err)
				source, err := query.Source()
				require.NoError(t, err)
				got, err := json.MarshalIndent(source, "", "  ")
				require.NoError(t, err)
				snapshottest.Assert(t, filterSnapshots+"/"+snapshotName(test.name), string(got))
			})
			claimed[snapshotName(test.name)+".json"] = true
		}
	})
	assertEverySnapshotIsClaimed(t, filterSnapshots, claimed)
}

// snapshotName turns a case name into the file name holding its lowered query.
func snapshotName(testName string) string {
	var name strings.Builder
	for _, r := range strings.ToLower(testName) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			name.WriteRune(r)
		case name.Len() > 0 && !strings.HasSuffix(name.String(), "_"):
			name.WriteRune('_')
		default:
			// Leading and repeated punctuation contribute nothing.
		}
	}
	return strings.TrimSuffix(name.String(), "_")
}

// assertEverySnapshotIsClaimed fails on a snapshot file no case asked for, which is what a renamed
// or deleted case leaves behind. snapshottest's own orphan check cannot see these, because it looks
// for other spellings of the one subject it was asked about rather than at the directory.
// Regenerating removes them instead of reporting them.
func assertEverySnapshotIsClaimed(t *testing.T, dir string, claimed map[string]bool) {
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "reading %s", dir)
	for _, entry := range entries {
		if claimed[entry.Name()] {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if snapshottest.Regenerate {
			require.NoError(t, os.Remove(path))
			continue
		}
		assert.Fail(t, "stale snapshot",
			"%s belongs to no test case; run REGENERATE_SNAPSHOTS=true to remove it", path)
	}
}

// TestBuildFilterQueryRefused covers what this schema cannot answer. Every case is refused
// rather than approximated, and each refusal is one of the two sentinels the API layers turn
// into a 400 — ErrFilterUnsupported for a predicate this backend cannot serve,
// ErrFilterInvalid for one that is malformed however it is served.
//
// A predicate the builder can write is written with it. The rest are trees the builder will not
// produce — a typed constant, a malformed one, or an operator given the wrong arguments — and are
// written out term by term, which is also what a remote client can put on the wire.
func TestBuildFilterQueryRefused(t *testing.T) {
	var p builder.Predicate
	tests := []struct {
		name    string
		filter  *expression.Call
		wantErr error
		wantMsg string
	}{
		{
			name:    "an instrumentation scope's attributes, which the write path never writes",
			filter:  p.Scope().Attr("library.tier").Eq("core"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `does not index the attributes of the "scope" level`,
		},
		{
			name:    "a link's attributes, which a reference does not carry either",
			filter:  p.Link().Attr("k").Eq("v"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `does not index the attributes of the "link" level`,
		},
		{
			name:    "a built-in field this schema has no column for",
			filter:  p.Span().Kind.Eq("server"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `built-in field "kind" of the "span" level`,
		},
		{
			name:    "a built-in field of a level whose other fields are served",
			filter:  p.Link().TraceState.Eq("congo=t61rcWkgMzE"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `built-in field "traceState" of the "link" level`,
		},
		{
			name:    "exists on a built-in field this schema has no column for",
			filter:  p.Span().EndTime.Exists(),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `built-in field "endTime" of the "span" level`,
		},
		{
			name:    "ordering an attribute, which is indexed as a keyword",
			filter:  p.Span().Attr("http.response.size").Gt("500"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `indexes "http.response.size" as a keyword rather than a number`,
		},
		{
			name:    "ordering the operation name",
			filter:  p.Span().Name.Lte("m"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `indexes "name" as a keyword rather than a number`,
		},
		{
			name:    "a pattern over the duration, which is a number",
			filter:  p.Span().Duration.Matches("2.*"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `operator "regex" on a duration`,
		},
		{
			name:    "a constant that declares its type, which there is no typed storage to route to",
			filter:  p.Span().Attr("http.status_code").Eq(&expression.IntValue{Value: 500}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "an integer constant declares a type",
		},
		{
			name:    "two references, which this engine would need a script to compare",
			filter:  p.Span().Name.Eq(p.Span().Attr("http.route")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `it evaluates "eq" against a constant only`,
		},
		{
			name:    "a pattern using a Perl shorthand, which this engine reads as a letter",
			filter:  p.Span().Attr("http.status_code").Matches(`\d+`),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `it reads "\\d" as the literal character`,
		},
		{
			name:    "a pattern using a word shorthand",
			filter:  p.Span().Name.Matches(`GET \w+`),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `it reads "\\w" as the literal character`,
		},
		{
			name:    "a duration constant carrying nothing, which a finalized filter never holds",
			filter:  p.Span().Duration.Gt((*expression.DurationValue)(nil)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a duration constant declares a type",
		},
		{
			name:    "an untyped constant carrying nothing",
			filter:  p.Span().Duration.Gt((*expression.AnyValue)(nil)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "that operand declares a type",
		},
		{
			name:    "a timestamp constant carrying nothing, which a finalized filter never holds",
			filter:  p.Span().StartTime.Gt((*expression.TimestampValue)(nil)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a timestamp constant declares a type",
		},
		{
			name:    "an untyped constant carrying nothing, beside an instant",
			filter:  p.Event().Time.Gt((*expression.AnyValue)(nil)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "that operand declares a type",
		},
		{
			name:    "a list where a comparison takes one value",
			filter:  p.Span().Attr("http.route").Eq(&expression.List{Values: []string{"/cart"}}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "that operand declares a type",
		},
		{
			name:    "a list of doubles, refused for the reason a double constant is",
			filter:  p.Span().Attr("ratio").In(p.List(expression.ValueTypeDouble, 1.5)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a floating-point constant declares a type",
		},
		{
			name:    "a list of booleans, refused the same way",
			filter:  p.Span().Attr("ok").In(p.List(expression.ValueTypeBool, true)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a boolean constant declares a type",
		},
		{
			name:    "a timestamp constant beside an attribute, which is stored as text",
			filter:  p.Span().Attr("checkout.deadline").Eq(&expression.TimestampValue{Value: time.Unix(0, 0).UTC()}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a timestamp constant declares a type",
		},
		{
			name:    "a typed constant beside a keyword field, which holds only text",
			filter:  p.Span().TraceID.Eq(&expression.IntValue{Value: 3841}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "an integer constant declares a type",
		},
		{
			name:    "a string where the duration belongs, which carries no unit to read",
			filter:  p.Span().Duration.Gt(p.Text("2s")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a string constant declares a type",
		},
		{
			name:    "a duration where an instant belongs",
			filter:  p.Span().StartTime.Gt(&expression.DurationValue{Value: time.Second}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a duration constant declares a type",
		},
		{
			name:    "a pattern over the start time, which is a number",
			filter:  p.Span().StartTime.Matches("2026.*"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `operator "regex" on a timestamp`,
		},
		{
			name:    "a pattern over an event's time, the other column holding one",
			filter:  p.Event().Time.Matches("2026.*"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `operator "regex" on a timestamp`,
		},
		{
			name:    "a boolean where the duration belongs",
			filter:  p.Span().Duration.Gt(&expression.BoolValue{Value: true}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a boolean constant declares a type",
		},
		{
			name:    "a typed list, whose type applies to every member",
			filter:  p.Span().Attr("http.status_code").In(p.List(expression.ValueTypeInt, 500)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "an integer constant declares a type",
		},
		{
			name:    "the some quantifier, whose correlated matching is not implemented",
			filter:  p.Some(p.Event(), p.Event().Name.Eq("exception")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `operator "some"`,
		},
		{
			name:    "an operator this build does not know",
			filter:  p.Compare("json_extract", p.Span().Attr("input"), "$.a"),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `operator "json_extract"`,
		},
		{
			name:    "comparing two values read off the same span, which would need a script",
			filter:  p.Span().Attr("enduser.id").Ne(p.Resource().Attr("enduser.id")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `evaluates "ne" against a constant only`,
		},
		{
			// Compare routes membership through a list, so the builder cannot put a lone constant
			// where in expects one.
			name: "membership against something other than a list",
			filter: call(expression.OpIn,
				&expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				scalar("cart")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `evaluates "in" against a constant only`,
		},
		{
			name:    "a duration value with no unit",
			filter:  p.Span().Duration.Gt("2"),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"2" is not a duration such as "2s"`,
		},
		{
			// ne builds the presence test before the comparison, so this reaches the second of
			// the two and is refused by it.
			name:    "a duration value with no unit, negated",
			filter:  p.Span().Duration.Ne("2"),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"2" is not a duration such as "2s"`,
		},
		{
			name:    "a list member that is not a duration",
			filter:  p.Span().Duration.NotIn("2s", "later"),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `invalid duration "later"`,
		},
		{
			name:    "an instant that is not RFC 3339",
			filter:  p.Span().StartTime.Gt("yesterday"),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"yesterday" is not an instant such as`,
		},
		{
			name:    "an instant that is not RFC 3339, negated",
			filter:  p.Event().Time.Ne("yesterday"),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"yesterday" is not an instant such as`,
		},
		{
			name:    "a list member that is not an instant",
			filter:  p.Span().StartTime.NotIn("2026-08-16T18:56:20Z", "later"),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `cannot parse "later"`,
		},
		{
			name:    "a comparison with the wrong number of arguments",
			filter:  call(expression.OpEq, spanAttr("k")),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"eq" cannot take 1 arguments`,
		},
		{
			name: "not, which negates exactly one predicate",
			filter: call(expression.OpNot,
				call(expression.OpEq, spanAttr("a"), scalar("1")),
				call(expression.OpEq, spanAttr("b"), scalar("2"))),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"not" cannot take 2 arguments`,
		},
		{
			name:    "a combinator with no arguments",
			filter:  call(expression.OpAnd),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"and" cannot take 0 arguments`,
		},
		{
			name:    "a combinator given a value where a predicate belongs",
			filter:  call(expression.OpAnd, spanAttr("k"), scalar("v")),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"and" combines predicates, not values`,
		},
		{
			name:    "exists given a constant, which reads nothing off the span",
			filter:  call(expression.OpExists, scalar("k")),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"exists" reads a value on the span`,
		},
		{
			name:    "exists with the wrong number of arguments",
			filter:  call(expression.OpExists, spanAttr("a"), spanAttr("b")),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"exists" cannot take 2 arguments`,
		},
		{
			name:    "membership against an empty list, which nothing can satisfy",
			filter:  p.Compare(expression.OpIn, p.Resource().Service, &expression.List{}),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"in" compares against an empty list`,
		},
		{
			name: "membership with the wrong number of arguments",
			filter: call(expression.OpNotIn,
				&expression.List{Values: []string{"a"}}),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"not_in" cannot take 1 arguments`,
		},
	}
	withSpanReader(t, func(r *spanReaderTest) {
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				query, err := r.reader.buildFilterQuery(test.filter)
				assert.Nil(t, query)
				require.ErrorIs(t, err, test.wantErr)
				assert.Contains(t, err.Error(), test.wantMsg)
			})
		}
	})
}

// TestBuildFindTraceIDsQueryWithFilter checks how the filter joins the search: as one more
// must clause beside the time range, which is the only other clause a filter query carries
// because the query service keeps the legacy predicate fields out of it.
func TestBuildFindTraceIDsQueryWithFilter(t *testing.T) {
	var p builder.Predicate
	withSpanReader(t, func(r *spanReaderTest) {
		start := time.Time{}
		end := time.Time{}.Add(time.Second)
		query, err := r.reader.buildFindTraceIDsQuery(dbmodel.TraceQueryParameters{
			StartTimeMin: start,
			StartTimeMax: end,
			Filter:       p.Resource().Service.Eq("cart"),
		})
		require.NoError(t, err)
		got, err := query.Source()
		require.NoError(t, err)
		want, err := esquery.NewBoolQuery().Must(
			r.reader.buildStartTimeQuery(start, end),
			esquery.NewTermQuery(serviceNameField, "cart"),
		).Source()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

// TestFindTraceIDsRefusesUnservableFilter checks that a refusal reaches the caller instead of
// a search running without the predicate it could not lower, which would answer a different
// question than the one asked.
func TestFindTraceIDsRefusesUnservableFilter(t *testing.T) {
	var p builder.Predicate
	withSpanReader(t, func(r *spanReaderTest) {
		now := time.Now()
		_, err := r.reader.FindTraceIDs(context.Background(), dbmodel.TraceQueryParameters{
			StartTimeMin: now,
			StartTimeMax: now.Add(time.Hour),
			Filter:       p.Span().Kind.Eq("server"),
		})
		require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
	})
}

// TestBuildFilterQueryRefusalFromWithin checks that a refusal deep in the tree is the
// refusal the caller sees, rather than being lost as a partially built query.
func TestBuildFilterQueryRefusalFromWithin(t *testing.T) {
	var p builder.Predicate
	unservable := p.Link().Attr("k").Eq("v")
	servable := p.Resource().Service.Eq("cart")
	for _, filter := range []*expression.Call{
		p.And(servable, unservable),
		p.Or(servable, unservable),
		p.Not(unservable),
		p.And(servable, p.Or(servable, unservable)),
		// The negated leaves build the positive comparison and the presence test in turn, so
		// either of them can be the one that refuses.
		p.Link().Attr("k").Ne("v"),
		p.Link().Attr("k").NotIn("v"),
	} {
		withSpanReader(t, func(r *spanReaderTest) {
			query, err := r.reader.buildFilterQuery(filter)
			assert.Nil(t, query)
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Contains(t, err.Error(), `does not index the attributes of the "link" level`)
		})
	}
}
