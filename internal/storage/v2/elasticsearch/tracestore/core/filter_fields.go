// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// indexedField locates built-in identity and timestamp fields. The nested path belongs
// to the field, so existence and value predicates enter the same collection.
func indexedField(ref reference) (field, path string) {
	switch {
	case ref.isField(expression.LevelSpan, expression.SpanFieldTraceID):
		return traceIDField, ""
	case ref.isField(expression.LevelSpan, expression.SpanFieldSpanID):
		return spanIDField, ""
	case ref.isField(expression.LevelSpan, expression.SpanFieldStartTime):
		return startTimeMillisField, ""
	case ref.isField(expression.LevelLink, expression.LinkFieldTraceID):
		return "references.traceID", "references"
	case ref.isField(expression.LevelLink, expression.LinkFieldSpanID):
		return "references.spanID", "references"
	case ref.isField(expression.LevelEvent, expression.EventFieldTime):
		return "logs.timestamp", "logs"
	default:
		return "", ""
	}
}

func inCollection(path string, query esquery.Query) esquery.Query {
	if path == "" {
		return query
	}
	return esquery.NewNestedQuery(path, query)
}

// scopeIdentity locates the scope identity in span tags. Scope attributes are dropped
// by the converter; only these two built-in fields may use the span's attribute storage.
func scopeIdentity(ref reference) (reference, bool) {
	switch {
	case ref.isField(expression.LevelScope, expression.ScopeFieldName):
		return reference{level: expression.LevelSpan, name: "otel.scope.name", attribute: true}, true
	case ref.isField(expression.LevelScope, expression.ScopeFieldVersion):
		return reference{level: expression.LevelSpan, name: "otel.scope.version", attribute: true}, true
	default:
		return reference{}, false
	}
}

func indexedFieldComparison(ref reference, field, path string, op expression.Operator, value expression.Expression) (esquery.Query, error) {
	if ref.isField(expression.LevelSpan, expression.SpanFieldStartTime) || ref.isField(expression.LevelEvent, expression.EventFieldTime) {
		query, err := timestampComparison(field, op, value)
		if err != nil {
			return nil, err
		}
		return inCollection(path, query), nil
	}
	text, err := constantText(value)
	if err != nil {
		return nil, err
	}
	if ref.level == expression.LevelLink && op == expression.OpRegex {
		return nil, fmt.Errorf("%w: exact link identity matching does not support the operator %q", tracestore.ErrFilterUnsupported, op)
	}
	if op != expression.OpRegex {
		// The v2 converter writes fixed-width hexadecimal OTLP IDs. Validate literals
		// here too, since callers can reach storage without query-service validation.
		width := 16
		if ref.name == expression.SpanFieldTraceID {
			width = 32
		}
		if _, err := hex.DecodeString(text); err != nil || len(text) != width {
			return nil, fmt.Errorf("%w: %s.%s requires a %d-digit hexadecimal ID", tracestore.ErrFilterInvalid, ref.level, ref.name, width)
		}
		text = strings.ToLower(text)
	}
	match, err := indexedTextMatch(op, ref, text)
	if err != nil {
		return nil, err
	}
	candidate := inCollection(path, match(field))
	if ref.level == expression.LevelLink {
		return exactLinkQuery(candidate, ref.name, op, text), nil
	}
	return candidate, nil
}

// exactLinkQuery first uses the indexed nested value as a candidate prefilter, then
// checks the original span document. The write path may put a synthetic CHILD_OF
// parent in references, so a nested query alone cannot tell it from a link. The
// script runs at the root document and can compare a reference with parentSpanID.
//
// A real CHILD_OF link that the legacy writer coalesced with that synthetic parent
// is indistinguishable from a parent-only span in existing data. The script follows
// the reader's recoverable representation and deliberately does not claim otherwise.
func exactLinkQuery(candidate esquery.Query, field string, op expression.Operator, value string) esquery.Query {
	return esquery.NewScriptScoreQuery(candidate, exactLinkScript, map[string]any{
		"field": field,
		"op":    string(op),
		"value": value,
	}, 1)
}

const exactLinkScript = `
def source = params._source;
if (source == null) {
  throw new IllegalArgumentException('link filters require _source');
}
def references = source.references;
if (references == null) {
  throw new IllegalArgumentException('link filters require _source references');
}
def parentTraceID = source.traceID;
def parentSpanID = source.parentSpanID;
if (parentSpanID == null || parentSpanID == '') {
  def followsFrom = null;
  for (def reference : references) {
    if (reference.traceID != parentTraceID) continue;
    if (reference.refType == 'CHILD_OF') {
      parentSpanID = reference.spanID;
      break;
    }
    if (followsFrom == null && reference.refType == 'FOLLOWS_FROM') followsFrom = reference.spanID;
  }
  if (parentSpanID == null || parentSpanID == '') parentSpanID = followsFrom;
}
for (def reference : references) {
  if (reference.refType == 'CHILD_OF' && reference.traceID == parentTraceID && reference.spanID == parentSpanID) continue;
  def actual = reference[params.field];
  if (actual == null) continue;
  if (params.op == 'exists') return 1;
  if (params.op == 'eq' && actual == params.value) return 1;
  if (params.op == 'gt' && actual.compareTo(params.value) > 0) return 1;
  if (params.op == 'gte' && actual.compareTo(params.value) >= 0) return 1;
  if (params.op == 'lt' && actual.compareTo(params.value) < 0) return 1;
  if (params.op == 'lte' && actual.compareTo(params.value) <= 0) return 1;
}
return 0;
`

// indexedTextMatch orders fields whose values are known to be text. It must not use
// attributeValueMatch, which orders an arbitrary attribute's numeric sub-field.
func indexedTextMatch(op expression.Operator, ref reference, text string) (valueMatch, error) {
	switch op {
	case expression.OpEq:
		return termMatch(text), nil
	case expression.OpRegex:
		return forThisEngine(text)
	case expression.OpGt:
		return func(field string) esquery.Query { return esquery.NewRangeQuery(field).Gt(text) }, nil
	case expression.OpGte:
		return func(field string) esquery.Query { return esquery.NewRangeQuery(field).Gte(text) }, nil
	case expression.OpLt:
		return func(field string) esquery.Query { return esquery.NewRangeQuery(field).Lt(text) }, nil
	case expression.OpLte:
		return func(field string) esquery.Query { return esquery.NewRangeQuery(field).Lte(text) }, nil
	default:
		return nil, errUnorderedValue(op, ref)
	}
}

func timestampComparison(field string, op expression.Operator, value expression.Expression) (esquery.Query, error) {
	if op != expression.OpEq && !ordersValues(op) {
		return nil, fmt.Errorf("%w: it does not support the operator %q on a timestamp", tracestore.ErrFilterUnsupported, op)
	}
	instant, err := timestampConstant(value)
	if err != nil {
		return nil, err
	}
	// Match the converter and buildStartTimeQuery: event times are epoch microseconds,
	// while span start-time filters use the date index and truncate to milliseconds.
	bound := model.TimeAsEpochMicroseconds(instant)
	if field == startTimeMillisField {
		bound /= 1000
	}
	switch op {
	case expression.OpEq:
		return esquery.NewTermQuery(field, bound), nil
	case expression.OpGt:
		return esquery.NewRangeQuery(field).Gt(bound), nil
	case expression.OpGte:
		return esquery.NewRangeQuery(field).Gte(bound), nil
	case expression.OpLt:
		return esquery.NewRangeQuery(field).Lt(bound), nil
	default: // OpLte, after the operator check above.
		return esquery.NewRangeQuery(field).Lte(bound), nil
	}
}

func timestampConstant(value expression.Expression) (time.Time, error) {
	if text, ok := value.(*expression.AnyValue); ok && text != nil {
		parsed, err := tracestore.ReadFilterConstant(expression.FieldTypeTimestamp, text.Value)
		if err != nil {
			return time.Time{}, fmt.Errorf("%w: invalid timestamp %q: %w", tracestore.ErrFilterInvalid, text.Value, err)
		}
		value = parsed
	}
	stamp, ok := value.(*expression.TimestampValue)
	if !ok || stamp == nil {
		return time.Time{}, errTypedConstant(value)
	}
	// The storage conversion uses UnixNano followed by an unsigned conversion. Outside
	// this interval it would overflow or wrap instead of describing the requested instant.
	if stamp.Value.Before(time.Unix(0, 0)) || stamp.Value.After(time.Unix(0, math.MaxInt64)) {
		return time.Time{}, fmt.Errorf("%w: timestamp is outside the storage conversion's epoch-nanosecond range", tracestore.ErrFilterUnsupported)
	}
	return stamp.Value, nil
}
