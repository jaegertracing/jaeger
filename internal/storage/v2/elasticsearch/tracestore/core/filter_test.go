// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// spanAttr, resourceAttr, eventAttr, and unqualifiedAttr spell the reference kinds the
// tests below compare against, so a case reads as the filter a caller would send.
func spanAttr(name string) *expression.AttributeRef {
	return &expression.AttributeRef{Key: name, Level: expression.LevelSpan}
}

func resourceAttr(name string) *expression.AttributeRef {
	return &expression.AttributeRef{Key: name, Level: expression.LevelResource}
}

func eventAttr(name string) *expression.AttributeRef {
	return &expression.AttributeRef{Key: name, Level: expression.LevelEvent}
}

func unqualifiedAttr(name string) *expression.AttributeRef {
	return &expression.AttributeRef{Key: name}
}

// scalar spells an untyped constant, which is what a caller writes and what this reader serves.
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
		expression.LevelEvent,
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
	assert.False(t, caps.SupportsLevel(expression.LevelScope))
	assert.False(t, caps.SupportsLevel(expression.LevelLink))
}

func TestBuildFilterQuery(t *testing.T) {
	tests := []struct {
		name   string
		filter *expression.Call
		want   string
	}{
		{
			name:   "unqualified attribute searches the span and resource levels",
			filter: call(expression.OpEq, unqualifiedAttr("http.status_code"), scalar("500")),
			want: `{"bool":{"should":[
				{"term":{"tag.http@status_code":"500"}},
				{"term":{"process.tag.http@status_code":"500"}},
				{"nested":{"path":"tags","query":{"bool":{"must":[
					{"term":{"tags.key":"http.status_code"}},
					{"term":{"tags.value":"500"}}]}}}},
				{"nested":{"path":"process.tags","query":{"bool":{"must":[
					{"term":{"process.tags.key":"http.status_code"}},
					{"term":{"process.tags.value":"500"}}]}}}}]}}`,
		},
		{
			name:   "span level searches only the span's own attributes",
			filter: call(expression.OpEq, spanAttr("component"), scalar("grpc")),
			want: `{"bool":{"should":[
				{"term":{"tag.component":"grpc"}},
				{"nested":{"path":"tags","query":{"bool":{"must":[
					{"term":{"tags.key":"component"}},
					{"term":{"tags.value":"grpc"}}]}}}}]}}`,
		},
		{
			name:   "resource level searches only the process attributes",
			filter: call(expression.OpEq, resourceAttr("deployment.environment"), scalar("staging")),
			want: `{"bool":{"should":[
				{"term":{"process.tag.deployment@environment":"staging"}},
				{"nested":{"path":"process.tags","query":{"bool":{"must":[
					{"term":{"process.tags.key":"deployment.environment"}},
					{"term":{"process.tags.value":"staging"}}]}}}}]}}`,
		},
		{
			name:   "event level has one location, so it needs no disjunction",
			filter: call(expression.OpEq, eventAttr("exception.type"), scalar("IOError")),
			want: `{"nested":{"path":"logs.fields","query":{"bool":{"must":[
				{"term":{"logs.fields.key":"exception.type"}},
				{"term":{"logs.fields.value":"IOError"}}]}}}}`,
		},
		{
			name:   "event.name reads the logs.fields entry the write path stores it in",
			filter: call(expression.OpEq, &expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}, scalar("exception")),
			want: `{"nested":{"path":"logs.fields","query":{"bool":{"must":[
				{"term":{"logs.fields.key":"event"}},
				{"term":{"logs.fields.value":"exception"}}]}}}}`,
		},
		{
			name:   "span.name is the operation name",
			filter: call(expression.OpEq, &expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, scalar("/api/v3/traces")),
			want:   `{"term":{"operationName":"/api/v3/traces"}}`,
		},
		{
			name:   "resource.service is the service name",
			filter: call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("cart")),
			want:   `{"term":{"process.serviceName":"cart"}}`,
		},
		{
			name:   "span.duration compares microseconds against a value carrying its unit",
			filter: call(expression.OpGt, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("2s")),
			want:   `{"range":{"duration":{"gt":2000000}}}`,
		},
		{
			name:   "gte on the duration",
			filter: call(expression.OpGte, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("1500ms")),
			want:   `{"range":{"duration":{"gte":1500000}}}`,
		},
		{
			name:   "lt on the duration",
			filter: call(expression.OpLt, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("1m")),
			want:   `{"range":{"duration":{"lt":60000000}}}`,
		},
		{
			name:   "lte on the duration",
			filter: call(expression.OpLte, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("500us")),
			want:   `{"range":{"duration":{"lte":500}}}`,
		},
		{
			name:   "eq on the duration",
			filter: call(expression.OpEq, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("3s")),
			want:   `{"term":{"duration":3000000}}`,
		},
		{
			name: "a duration constant, which is what a finalized filter carries",
			filter: call(expression.OpGt,
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.DurationValue{Value: 2 * time.Second}),
			want: `{"range":{"duration":{"gt":2000000}}}`,
		},
		{
			name:   "regex matches anywhere in the value, which this engine needs told",
			filter: call(expression.OpRegex, &expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, scalar("GET .*")),
			want:   `{"regexp":{"operationName":{"value":".*(GET .*).*"}}}`,
		},
		{
			name:   "regex on an attribute reaches every location the attribute lives in",
			filter: call(expression.OpRegex, spanAttr("http.route"), scalar("/api/.*")),
			want: `{"bool":{"should":[
				{"regexp":{"tag.http@route":{"value":".*(/api/.*).*"}}},
				{"nested":{"path":"tags","query":{"bool":{"must":[
					{"term":{"tags.key":"http.route"}},
					{"regexp":{"tags.value":{"value":".*(/api/.*).*"}}}]}}}}]}}`,
		},
		{
			name:   "an alternation is grouped, so the wildcards apply to the whole pattern",
			filter: call(expression.OpRegex, spanAttr("http.route"), scalar("cart|checkout")),
			want: `{"bool":{"should":[
				{"regexp":{"tag.http@route":{"value":".*(cart|checkout).*"}}},
				{"nested":{"path":"tags","query":{"bool":{"must":[
					{"term":{"tags.key":"http.route"}},
					{"regexp":{"tags.value":{"value":".*(cart|checkout).*"}}}]}}}}]}}`,
		},
		{
			name:   "exists tests the key alone",
			filter: call(expression.OpExists, eventAttr("exception.stacktrace")),
			want: `{"nested":{"path":"logs.fields",
				"query":{"term":{"logs.fields.key":"exception.stacktrace"}}}}`,
		},
		{
			name:   "exists on an attribute tests the key in every location it may live in",
			filter: call(expression.OpExists, spanAttr("http.route")),
			want: `{"bool":{"should":[
				{"exists":{"field":"tag.http@route"}},
				{"nested":{"path":"tags","query":{"term":{"tags.key":"http.route"}}}}]}}`,
		},
		{
			name:   "exists on event.name tests the key the write path stores it under",
			filter: call(expression.OpExists, &expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}),
			want: `{"nested":{"path":"logs.fields",
				"query":{"term":{"logs.fields.key":"event"}}}}`,
		},
		{
			name:   "exists on a built-in field tests its own field",
			filter: call(expression.OpExists, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}),
			want:   `{"exists":{"field":"duration"}}`,
		},
		{
			name:   "exists on the operation name",
			filter: call(expression.OpExists, &expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}),
			want:   `{"exists":{"field":"operationName"}}`,
		},
		{
			name:   "exists on the service name",
			filter: call(expression.OpExists, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}),
			want:   `{"exists":{"field":"process.serviceName"}}`,
		},
		{
			name: "and becomes the must clause",
			filter: call(expression.OpAnd,
				call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("cart")),
				call(expression.OpGt, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("2s"))),
			want: `{"bool":{"must":[
				{"term":{"process.serviceName":"cart"}},
				{"range":{"duration":{"gt":2000000}}}]}}`,
		},
		{
			// A one-argument conjunction is not a filter a caller can send through the query
			// service, which validates arity first, but it can arrive from a remote-storage
			// client. It means the predicate it wraps, and lowers to it.
			name: "a conjunction of one predicate is that predicate",
			filter: call(expression.OpAnd,
				call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("cart"))),
			want: `{"term":{"process.serviceName":"cart"}}`,
		},
		{
			name: "or becomes the should clause",
			filter: call(expression.OpOr,
				call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("cart")),
				call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("checkout"))),
			want: `{"bool":{"should":[
				{"term":{"process.serviceName":"cart"}},
				{"term":{"process.serviceName":"checkout"}}]}}`,
		},
		{
			name: "not becomes the must_not clause, which also matches a span missing the reference",
			filter: call(expression.OpNot,
				call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("healthcheck"))),
			want: `{"bool":{"must_not":{"term":{"process.serviceName":"healthcheck"}}}}`,
		},
		{
			name: "nesting composes to nested bool queries",
			filter: call(expression.OpAnd,
				call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("cart")),
				call(expression.OpOr,
					call(expression.OpEq, &expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, scalar("a")),
					call(expression.OpNot,
						call(expression.OpEq, &expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, scalar("b"))))),
			want: `{"bool":{"must":[
				{"term":{"process.serviceName":"cart"}},
				{"bool":{"should":[
					{"term":{"operationName":"a"}},
					{"bool":{"must_not":{"term":{"operationName":"b"}}}}]}}]}}`,
		},
		{
			name:   "ne requires the reference to be present, so an absent one does not match",
			filter: call(expression.OpNe, eventAttr("exception.type"), scalar("IOError")),
			want: `{"bool":{
				"must":{"nested":{"path":"logs.fields",
					"query":{"term":{"logs.fields.key":"exception.type"}}}},
				"must_not":{"nested":{"path":"logs.fields","query":{"bool":{"must":[
					{"term":{"logs.fields.key":"exception.type"}},
					{"term":{"logs.fields.value":"IOError"}}]}}}}}}`,
		},
		{
			name:   "ne on a built-in field guards on the field's own presence",
			filter: call(expression.OpNe, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("cart")),
			want: `{"bool":{
				"must":{"exists":{"field":"process.serviceName"}},
				"must_not":{"term":{"process.serviceName":"cart"}}}}`,
		},
		{
			name: "in is the disjunction of equalities it stands for",
			filter: call(expression.OpIn, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				&expression.List{Values: []string{"cart", "checkout"}}),
			want: `{"bool":{"should":[
				{"term":{"process.serviceName":"cart"}},
				{"term":{"process.serviceName":"checkout"}}]}}`,
		},
		{
			name: "a single-member list needs no disjunction",
			filter: call(expression.OpIn, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				&expression.List{Values: []string{"cart"}}),
			want: `{"term":{"process.serviceName":"cart"}}`,
		},
		{
			name: "not_in requires the reference to be present, like ne",
			filter: call(expression.OpNotIn, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				&expression.List{Values: []string{"cart", "checkout"}}),
			want: `{"bool":{
				"must":{"exists":{"field":"process.serviceName"}},
				"must_not":{"bool":{"should":[
					{"term":{"process.serviceName":"cart"}},
					{"term":{"process.serviceName":"checkout"}}]}}}}`,
		},
		{
			name:   "error=true matches the tag the write path records",
			filter: call(expression.OpEq, spanAttr("error"), scalar("true")),
			want: `{"bool":{"should":[
				{"term":{"tag.error":"true"}},
				{"nested":{"path":"tags","query":{"bool":{"must":[
					{"term":{"tags.key":"error"}},
					{"term":{"tags.value":"true"}}]}}}}]}}`,
		},
		{
			name:   "error=false excludes error=true, because a span that succeeded carries no tag",
			filter: call(expression.OpEq, spanAttr("error"), scalar("false")),
			want: `{"bool":{"must_not":{"bool":{"should":[
				{"term":{"tag.error":"true"}},
				{"nested":{"path":"tags","query":{"bool":{"must":[
					{"term":{"tags.key":"error"}},
					{"term":{"tags.value":"true"}}]}}}}]}}}}`,
		},
		{
			name:   "an unqualified error=0 is read the same way as error=false",
			filter: call(expression.OpEq, unqualifiedAttr("error"), scalar("0")),
			want: `{"bool":{"must_not":{"bool":{"should":[
				{"term":{"tag.error":"true"}},
				{"term":{"process.tag.error":"true"}},
				{"nested":{"path":"tags","query":{"bool":{"must":[
					{"term":{"tags.key":"error"}},
					{"term":{"tags.value":"true"}}]}}}},
				{"nested":{"path":"process.tags","query":{"bool":{"must":[
					{"term":{"process.tags.key":"error"}},
					{"term":{"process.tags.value":"true"}}]}}}}]}}}}`,
		},
		{
			name:   "a non-boolean error value keeps its literal match",
			filter: call(expression.OpEq, spanAttr("error"), scalar("oops")),
			want: `{"bool":{"should":[
				{"term":{"tag.error":"oops"}},
				{"nested":{"path":"tags","query":{"bool":{"must":[
					{"term":{"tags.key":"error"}},
					{"term":{"tags.value":"oops"}}]}}}}]}}`,
		},
		{
			name:   "an error tag at the resource level is an ordinary attribute",
			filter: call(expression.OpEq, resourceAttr("error"), scalar("false")),
			want: `{"bool":{"should":[
				{"term":{"process.tag.error":"false"}},
				{"nested":{"path":"process.tags","query":{"bool":{"must":[
					{"term":{"process.tags.key":"error"}},
					{"term":{"process.tags.value":"false"}}]}}}}]}}`,
		},
	}
	withSpanReader(t, func(r *spanReaderTest) {
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				query, err := r.reader.buildFilterQuery(test.filter)
				require.NoError(t, err)
				source, err := query.Source()
				require.NoError(t, err)
				got, err := json.Marshal(source)
				require.NoError(t, err)
				assert.JSONEq(t, test.want, string(got))
			})
		}
	})
}

// TestBuildFilterQueryRefused covers what this schema cannot answer. Every case is refused
// rather than approximated, and each refusal is one of the two sentinels the API layers turn
// into a 400 — ErrFilterUnsupported for a predicate this backend cannot serve,
// ErrFilterInvalid for one that is malformed however it is served.
func TestBuildFilterQueryRefused(t *testing.T) {
	tests := []struct {
		name    string
		filter  *expression.Call
		wantErr error
		wantMsg string
	}{
		{
			name: "the scope level is folded into the span's own tags",
			filter: call(expression.OpEq,
				&expression.AttributeRef{Key: "otel.scope.name", Level: expression.LevelScope},
				scalar("lib")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `does not index the "scope" level`,
		},
		{
			name: "link attributes are not indexed at all",
			filter: call(expression.OpEq,
				&expression.AttributeRef{Key: "k", Level: expression.LevelLink},
				scalar("v")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `does not index the "link" level`,
		},
		{
			name: "a built-in field this schema has no field for",
			filter: call(expression.OpEq,
				&expression.FieldRef{Name: "kind", Level: expression.LevelSpan},
				scalar("server")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `built-in field "kind" of the "span" level`,
		},
		{
			name: "exists on a built-in field this schema has no field for",
			filter: call(expression.OpExists,
				&expression.FieldRef{Name: "traceID", Level: expression.LevelLink}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `built-in field "traceID" of the "link" level`,
		},
		{
			name:    "ordering an attribute, which is indexed as a keyword",
			filter:  call(expression.OpGt, spanAttr("http.response.size"), scalar("500")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `indexes "http.response.size" as a keyword rather than a number`,
		},
		{
			name:    "ordering the operation name",
			filter:  call(expression.OpLte, &expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan}, scalar("m")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `indexes "name" as a keyword rather than a number`,
		},
		{
			name:    "a pattern over the duration, which is a number",
			filter:  call(expression.OpRegex, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("2.*")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `operator "regex" on a duration`,
		},
		{
			name: "a constant that declares its type, which there is no typed storage to route to",
			filter: call(expression.OpEq, spanAttr("http.status_code"),
				&expression.IntValue{Value: 500}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "an integer constant declares a type",
		},
		{
			name: "two references, which this engine would need a script to compare",
			filter: call(expression.OpEq,
				&expression.FieldRef{Name: expression.SpanFieldName, Level: expression.LevelSpan},
				spanAttr("http.route")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `it evaluates "eq" against a constant only`,
		},
		{
			name: "a string constant, whose declared type this schema cannot route to",
			filter: call(expression.OpEq, spanAttr("http.route"),
				&expression.StringValue{Value: "/cart"}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a string constant declares a type",
		},
		{
			name: "a duration constant carrying nothing, which a finalized filter never holds",
			filter: call(expression.OpGt,
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				(*expression.DurationValue)(nil)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a duration constant declares a type",
		},
		{
			name: "an untyped constant carrying nothing",
			filter: call(expression.OpGt,
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				(*expression.AnyValue)(nil)),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "that operand declares a type",
		},
		{
			name: "a list where a comparison takes one value",
			filter: call(expression.OpEq, spanAttr("http.route"),
				&expression.List{Values: []string{"/cart"}}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "that operand declares a type",
		},
		{
			name: "a list of doubles, refused for the reason a double constant is",
			filter: call(expression.OpIn, spanAttr("ratio"),
				&expression.List{Values: []string{"1.5"}, Type: expression.ValueTypeDouble}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a floating-point constant declares a type",
		},
		{
			name: "a list of booleans, refused the same way",
			filter: call(expression.OpIn, spanAttr("ok"),
				&expression.List{Values: []string{"true"}, Type: expression.ValueTypeBool}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a boolean constant declares a type",
		},
		{
			name: "a list of strings, whose declared text this schema cannot tell from a number",
			filter: call(expression.OpIn, spanAttr("http.route"),
				&expression.List{Values: []string{"/cart"}, Type: expression.ValueTypeString}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a string constant declares a type",
		},
		{
			name: "a timestamp constant, which no field here holds",
			filter: call(expression.OpGt,
				&expression.FieldRef{Name: expression.SpanFieldStartTime, Level: expression.LevelSpan},
				&expression.TimestampValue{Value: time.Unix(0, 0).UTC()}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a timestamp constant declares a type",
		},
		{
			name: "a boolean where the duration belongs",
			filter: call(expression.OpGt,
				&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.BoolValue{Value: true}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "a boolean constant declares a type",
		},
		{
			name: "a typed list, whose type applies to every member",
			filter: call(expression.OpIn, spanAttr("http.status_code"),
				&expression.List{Values: []string{"500"}, Type: expression.ValueTypeInt}),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: "an integer constant declares a type",
		},
		{
			name: "the some quantifier, whose correlated matching is not implemented",
			filter: call(expression.OpSome,
				&expression.NestedRef{Level: expression.LevelEvent},
				call(expression.OpEq, &expression.FieldRef{Name: expression.EventFieldName, Level: expression.LevelEvent}, scalar("exception"))),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `operator "some"`,
		},
		{
			name:    "an operator this build does not know",
			filter:  call("json_extract", spanAttr("input"), scalar("$.a")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `operator "json_extract"`,
		},
		{
			name:    "comparing two values read off the same span, which would need a script",
			filter:  call(expression.OpNe, spanAttr("enduser.id"), resourceAttr("enduser.id")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `evaluates "ne" against a constant only`,
		},
		{
			name: "membership against something other than a list",
			filter: call(expression.OpIn, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				scalar("cart")),
			wantErr: tracestore.ErrFilterUnsupported,
			wantMsg: `evaluates "in" against a constant only`,
		},
		{
			name:    "a duration value with no unit",
			filter:  call(expression.OpGt, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("2")),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"2" is not a duration such as "2s"`,
		},
		{
			// ne builds the presence test before the comparison, so this reaches the second of
			// the two and is refused by it.
			name:    "a duration value with no unit, negated",
			filter:  call(expression.OpNe, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan}, scalar("2")),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"2" is not a duration such as "2s"`,
		},
		{
			name: "a list member that is not a duration",
			filter: call(expression.OpNotIn, &expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
				&expression.List{Values: []string{"2s", "later"}}),
			wantErr: tracestore.ErrFilterInvalid,
			wantMsg: `"later" is not a duration such as "2s"`,
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
			name: "membership against an empty list, which nothing can satisfy",
			filter: call(expression.OpIn, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource},
				&expression.List{}),
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
	withSpanReader(t, func(r *spanReaderTest) {
		start := time.Time{}
		end := time.Time{}.Add(time.Second)
		query, err := r.reader.buildFindTraceIDsQuery(dbmodel.TraceQueryParameters{
			StartTimeMin: start,
			StartTimeMax: end,
			Filter:       call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("cart")),
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
	withSpanReader(t, func(r *spanReaderTest) {
		now := time.Now()
		_, err := r.reader.FindTraceIDs(context.Background(), dbmodel.TraceQueryParameters{
			StartTimeMin: now,
			StartTimeMax: now.Add(time.Hour),
			Filter: call(expression.OpEq,
				&expression.FieldRef{Name: "kind", Level: expression.LevelSpan},
				scalar("server")),
		})
		require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
	})
}

// TestBuildFilterQueryRefusalFromWithin checks that a refusal deep in the tree is the
// refusal the caller sees, rather than being lost as a partially built query.
func TestBuildFilterQueryRefusalFromWithin(t *testing.T) {
	unservable := call(expression.OpEq,
		&expression.AttributeRef{Key: "k", Level: expression.LevelLink},
		scalar("v"))
	servable := call(expression.OpEq, &expression.FieldRef{Name: expression.ResourceFieldService, Level: expression.LevelResource}, scalar("cart"))
	for _, filter := range []*expression.Call{
		call(expression.OpAnd, servable, unservable),
		call(expression.OpOr, servable, unservable),
		call(expression.OpNot, unservable),
		call(expression.OpAnd, servable, call(expression.OpOr, servable, unservable)),
		// The negated leaves build the positive comparison and the presence test in turn, so
		// either of them can be the one that refuses.
		call(expression.OpNe,
			&expression.AttributeRef{Key: "k", Level: expression.LevelLink},
			scalar("v")),
		call(expression.OpNotIn,
			&expression.AttributeRef{Key: "k", Level: expression.LevelLink},
			&expression.List{Values: []string{"v"}}),
	} {
		withSpanReader(t, func(r *spanReaderTest) {
			query, err := r.reader.buildFilterQuery(filter)
			assert.Nil(t, query)
			require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
			assert.Contains(t, err.Error(), `does not index the "link" level`)
		})
	}
}
