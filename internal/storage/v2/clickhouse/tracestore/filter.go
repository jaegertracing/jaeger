// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"
	"math"
	"regexp/syntax"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// minUnixNanoTime and maxUnixNanoTime are the earliest and the latest instant that a signed
// 64-bit count of Unix nanoseconds holds. The lowering passes a timestamp constant to
// ClickHouse as such a count.
var (
	minUnixNanoTime = time.Unix(0, math.MinInt64)
	maxUnixNanoTime = time.Unix(0, math.MaxInt64)
)

// FilterCapabilities declares the part of the RFC 0005 filter model this reader evaluates
// natively: the boolean combinators; eq, ne, exists, in and not_in over every level's attributes
// and over every built-in field that the spans table stores; the four ordered comparisons
// on those fields, except span.kind and span.status; and regex over string attributes and
// over the fields that hold text. The declaration is coarser than the
// lowering, as the elasticsearch backend's is: an ordered comparison on an attribute, on
// span.kind or on span.status, and a predicate on resource.schemaURL or scope.schemaURL, which
// have no column, are refused in the lowering, not here. The some
// operator and the text-search operators are follow-up work; a filter naming any of them is refused rather
// than approximated.
func FilterCapabilities() tracestore.FilterCapabilities {
	return tracestore.FilterCapabilities{
		Levels: []expression.Level{
			expression.LevelSpan,
			expression.LevelResource,
			expression.LevelScope,
			expression.LevelEvent,
			expression.LevelLink,
		},
		Operators: []expression.Operator{
			expression.OpAnd,
			expression.OpOr,
			expression.OpNot,
			expression.OpEq,
			expression.OpNe,
			expression.OpGt,
			expression.OpLt,
			expression.OpGte,
			expression.OpLte,
			expression.OpRegex,
			expression.OpExists,
			expression.OpIn,
			expression.OpNotIn,
		},
	}
}

// sqlComparisonOperators maps each comparison operator this lowering evaluates to its SQL
// spelling. Only a built-in field takes the four ordered ones. The ne operator has no entry,
// because its condition negates an equality. REGEXP is the operator form of the match
// function, which searches the value for a pattern in RE2 syntax.
var sqlComparisonOperators = map[expression.Operator]string{
	expression.OpEq:    "=",
	expression.OpGt:    ">",
	expression.OpLt:    "<",
	expression.OpGte:   ">=",
	expression.OpLte:   "<=",
	expression.OpRegex: "REGEXP",
}

// builtinFieldColumn is one built-in field's SQL column and the type its constant is read as.
type builtinFieldColumn struct {
	// column is the SQL expression of the value. For a field of an event or a link it is the
	// value of the one event or link that a lambda binds to x.
	column    string
	fieldType expression.FieldType
	// emptyMeansAbsent is set for a field that can be absent, which the column stores as an
	// empty string. No operator matches an absent value (RFC 0005 §5.3).
	emptyMeansAbsent bool
	// arrayColumn is the column that lookupBuiltinField fills in for a field of an event or a
	// link, which has one value per event or link of the span. It is empty for a field with
	// one value per span.
	arrayColumn string
}

// lookupBuiltinField returns the column of a built-in field. Every level is indexed, so a
// field with no entry is refused as an unsupported field, whichever level it belongs to.
func lookupBuiltinField(ref expression.FieldRef) (builtinFieldColumn, error) {
	mapping, ok := builtinFieldColumns[ref.Level][ref.Name]
	if !ok {
		return builtinFieldColumn{}, errUnsupportedField(ref)
	}
	mapping.arrayColumn = AttributeGroup{Level: ref.Level}.arrayColumn()
	return mapping, nil
}

// valueConditionSQL returns the condition that one value of the field is present and passes
// comparisonSQL, the SQL that follows the column, such as " = ?".
func (c builtinFieldColumn) valueConditionSQL(comparisonSQL string) string {
	if c.emptyMeansAbsent {
		return c.column + " != '' AND " + c.column + comparisonSQL
	}
	return c.column + comparisonSQL
}

// spanConditionSQL returns valueSQL, a condition on one value of the field, as a condition on
// the span: a span with events or links passes when one of them does.
func (c builtinFieldColumn) spanConditionSQL(valueSQL string) string {
	if c.arrayColumn == "" {
		return valueSQL
	}
	return "arrayExists(x -> " + valueSQL + ", s." + c.arrayColumn + ")"
}

// presenceSQL returns the condition that the span has a value of the field.
func (c builtinFieldColumn) presenceSQL() string {
	if c.emptyMeansAbsent {
		return c.spanConditionSQL(c.column + " != ''")
	}
	return c.spanConditionSQL("true")
}

// placeholderSQL returns the placeholder of a constant that the field is compared with.
func (c builtinFieldColumn) placeholderSQL() string {
	if c.fieldType == expression.FieldTypeTimestamp {
		return unixNanoPlaceholderSQL
	}
	return "?"
}

// appendCondition writes on its own line the condition that a value of the field passes
// comparisonSQL.
func (c builtinFieldColumn) appendCondition(q *sqlBuilder, indent int, comparisonSQL string) {
	valueSQL := c.valueConditionSQL(comparisonSQL)
	if c.emptyMeansAbsent && c.arrayColumn == "" {
		valueSQL = "(" + valueSQL + ")"
	}
	q.appendNewlineAndIndent(indent)
	q.WriteString(c.spanConditionSQL(valueSQL))
}

// appendNegatedCondition writes on its own line the condition of the ne and not_in
// operators: the span has a value of the field and none of its values passes comparisonSQL,
// which is the comparison of the operator that is negated.
func (c builtinFieldColumn) appendNegatedCondition(q *sqlBuilder, indent int, comparisonSQL string) {
	q.appendNewlineAndIndent(indent)
	if c.arrayColumn != "" {
		q.WriteString("(" + c.presenceSQL() + " AND NOT " + c.spanConditionSQL(c.valueConditionSQL(comparisonSQL)) + ")")
		return
	}
	noMatchSQL := "NOT (" + c.column + comparisonSQL + ")"
	if c.emptyMeansAbsent {
		noMatchSQL = "(" + c.presenceSQL() + " AND " + noMatchSQL + ")"
	}
	q.WriteString(noMatchSQL)
}

// builtinFieldColumns is the built-in fields this lowering compares, keyed by level and name.
// resource.schemaURL and scope.schemaURL have no column in the spans table, so they are
// refused (errUnsupportedField) rather than approximated.
var builtinFieldColumns = map[expression.Level]map[string]builtinFieldColumn{
	expression.LevelSpan: {
		expression.SpanFieldTraceID: {
			column:    "s.trace_id",
			fieldType: expression.FieldTypeString,
		},
		expression.SpanFieldSpanID: {
			column:    "s.id",
			fieldType: expression.FieldTypeString,
		},
		expression.SpanFieldParentSpanID: {
			column:           "s.parent_span_id",
			fieldType:        expression.FieldTypeString,
			emptyMeansAbsent: true,
		},
		expression.SpanFieldTraceState: {
			column:           "s.trace_state",
			fieldType:        expression.FieldTypeString,
			emptyMeansAbsent: true,
		},
		expression.SpanFieldName: {
			column:    "s.name",
			fieldType: expression.FieldTypeString,
		},
		expression.SpanFieldKind: {
			column:    "s.kind",
			fieldType: expression.FieldTypeSpanKind,
		},
		expression.SpanFieldStartTime: {
			column:    "s.start_time",
			fieldType: expression.FieldTypeTimestamp,
		},
		expression.SpanFieldEndTime: {
			column:    "addNanoseconds(s.start_time, s.duration)",
			fieldType: expression.FieldTypeTimestamp,
		},
		expression.SpanFieldDuration: {
			column:    "s.duration",
			fieldType: expression.FieldTypeDuration,
		},
		expression.SpanFieldStatus: {
			column:    "s.status_code",
			fieldType: expression.FieldTypeSpanStatus,
		},
		expression.SpanFieldStatusMessage: {
			column:           "s.status_message",
			fieldType:        expression.FieldTypeString,
			emptyMeansAbsent: true,
		},
	},
	expression.LevelResource: {
		expression.ResourceFieldService: {
			column:           "s.service_name",
			fieldType:        expression.FieldTypeString,
			emptyMeansAbsent: true,
		},
	},
	expression.LevelScope: {
		expression.ScopeFieldName: {
			column:           "s.scope_name",
			fieldType:        expression.FieldTypeString,
			emptyMeansAbsent: true,
		},
		expression.ScopeFieldVersion: {
			column:           "s.scope_version",
			fieldType:        expression.FieldTypeString,
			emptyMeansAbsent: true,
		},
	},
	expression.LevelEvent: {
		expression.EventFieldName: {
			column:    "x.name",
			fieldType: expression.FieldTypeString,
		},
		expression.EventFieldTime: {
			column:    "x.timestamp",
			fieldType: expression.FieldTypeTimestamp,
		},
		expression.EventFieldTimeSinceStart: {
			column:    "(toUnixTimestamp64Nano(x.timestamp) - toUnixTimestamp64Nano(s.start_time))",
			fieldType: expression.FieldTypeDuration,
		},
	},
	expression.LevelLink: {
		expression.LinkFieldTraceID: {
			column:    "x.trace_id",
			fieldType: expression.FieldTypeString,
		},
		expression.LinkFieldSpanID: {
			column:    "x.span_id",
			fieldType: expression.FieldTypeString,
		},
		expression.LinkFieldTraceState: {
			column:           "x.trace_state",
			fieldType:        expression.FieldTypeString,
			emptyMeansAbsent: true,
		},
	},
}

// lookupFilterAttributeMetadata looks up in attribute_metadata, in a single query, every
// attribute key of predicate whose lowering depends on the types that the key is stored with,
// so that one filter looks a key up once. A key with no metadata comes back absent.
func (r *Reader) lookupFilterAttributeMetadata(ctx context.Context, predicate *expression.Call) (attributeMetadata, error) {
	keys := pcommon.NewMap()
	collectMetadataKeys(predicate, keys)
	if keys.Len() == 0 {
		return attributeMetadata{}, nil
	}
	metadata, err := r.getAttributeMetadata(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("failed to get attribute metadata: %w", err)
	}
	return metadata, nil
}

// buildFilterCondition lowers predicate into the SQL WHERE tree, appended to q at indent, and
// returns the updated bind arguments. metadata is the attribute metadata already looked
// up for the whole tree by lookupFilterAttributeMetadata, so no part of the lowering queries
// attribute_metadata again. It mirrors the elasticsearch backend's buildFilterQuery: the filter
// arrives already checked against FilterCapabilities when it comes through the query service,
// but a remote-storage client can reach this reader without that check, so every refusal is
// made here too rather than assumed.
func (q *sqlBuilder) buildFilterCondition(
	indent int,
	args []any,
	metadata attributeMetadata,
	predicate *expression.Call,
) ([]any, error) {
	switch predicate.Op {
	case expression.OpAnd, expression.OpOr:
		return q.buildBooleanCondition(indent, args, metadata, predicate)
	case expression.OpNot:
		if len(predicate.Args) != 1 {
			return nil, errArity(predicate)
		}
		// A nil *expression.Call still asserts ok, since only the pointer is nil, so it is
		// checked separately or the recursion dereferences it instead of refusing.
		child, ok := predicate.Args[0].(*expression.Call)
		if !ok || child == nil {
			return nil, fmt.Errorf("%w: %q negates a predicate, not a value", tracestore.ErrFilterInvalid, predicate.Op)
		}
		q.appendNewlineAndIndent(indent)
		q.WriteString("NOT (")
		args, err := q.buildFilterCondition(indent+1, args, metadata, child)
		if err != nil {
			return nil, err
		}
		q.appendNewlineAndIndent(indent)
		q.WriteString(")")
		return args, nil
	case expression.OpEq, expression.OpNe, expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte:
		return q.buildComparisonCondition(indent, args, metadata, predicate)
	case expression.OpRegex:
		return q.buildRegexCondition(indent, args, predicate)
	case expression.OpExists:
		return q.buildExistsCondition(indent, args, metadata, predicate)
	case expression.OpIn, expression.OpNotIn:
		return q.buildMembershipCondition(indent, args, metadata, predicate)
	default:
		return nil, fmt.Errorf("%w: it does not support the operator %q", tracestore.ErrFilterUnsupported, predicate.Op)
	}
}

// buildBooleanCondition lowers and/or: each argument is itself a predicate, joined by the
// operator's SQL keyword and wrapped in parentheses so precedence survives nesting.
func (q *sqlBuilder) buildBooleanCondition(
	indent int,
	args []any,
	metadata attributeMetadata,
	predicate *expression.Call,
) ([]any, error) {
	if len(predicate.Args) == 0 {
		return nil, errArity(predicate)
	}
	sep := "AND"
	if predicate.Op == expression.OpOr {
		sep = "OR"
	}
	q.appendNewlineAndIndent(indent)
	q.WriteString("(")
	for i, arg := range predicate.Args {
		child, ok := arg.(*expression.Call)
		if !ok || child == nil {
			return nil, fmt.Errorf("%w: %q combines predicates, not values", tracestore.ErrFilterInvalid, predicate.Op)
		}
		if i > 0 {
			q.appendNewlineAndIndent(indent + 1)
			q.WriteString(sep)
		}
		var err error
		args, err = q.buildFilterCondition(indent+1, args, metadata, child)
		if err != nil {
			return nil, err
		}
	}
	q.appendNewlineAndIndent(indent)
	q.WriteString(")")
	return args, nil
}

// buildComparisonCondition dispatches a comparison to the built-in-field or attribute path by
// the shape of its first operand; a finalized filter puts the reference first (RFC 0005 §5.3)
// and a constant second.
func (q *sqlBuilder) buildComparisonCondition(
	indent int,
	args []any,
	metadata attributeMetadata,
	predicate *expression.Call,
) ([]any, error) {
	if len(predicate.Args) != 2 {
		return nil, errArity(predicate)
	}
	switch predicate.Args[1].(type) {
	case *expression.FieldRef, *expression.AttributeRef, *expression.NestedRef, *expression.Call:
		return nil, fmt.Errorf("%w: %q compares a field or attribute against a constant, not another reference or predicate",
			tracestore.ErrFilterInvalid, predicate.Op)
	}
	switch ref := predicate.Args[0].(type) {
	case *expression.FieldRef:
		return q.buildFieldComparison(indent, args, predicate.Op, *ref, predicate.Args[1])
	case *expression.AttributeRef:
		if predicate.Op != expression.OpEq && predicate.Op != expression.OpNe {
			return nil, fmt.Errorf("%w: it does not order attributes, so it cannot evaluate %q on %q",
				tracestore.ErrFilterUnsupported, predicate.Op, ref.Key)
		}
		constants := []expression.Expression{predicate.Args[1]}
		return q.buildAttributeMembership(indent, args, metadata, *ref, constants, predicate.Op == expression.OpNe)
	default:
		return nil, fmt.Errorf("%w: %q compares a field or attribute against a constant",
			tracestore.ErrFilterInvalid, predicate.Op)
	}
}

// buildFieldComparison lowers a comparison of a built-in field onto its column. Equality and
// inequality reach every mapped field. The ordered comparisons reach every field but span.kind
// and span.status, which hold words that have no order (RFC 0005 §5.4); a text field is ordered
// lexicographically.
func (q *sqlBuilder) buildFieldComparison(
	indent int,
	args []any,
	op expression.Operator,
	ref expression.FieldRef,
	valueExpr expression.Expression,
) ([]any, error) {
	mapping, err := lookupBuiltinField(ref)
	if err != nil {
		return nil, err
	}
	isOrderedComparison := op != expression.OpEq && op != expression.OpNe
	holdsWord := mapping.fieldType == expression.FieldTypeSpanKind || mapping.fieldType == expression.FieldTypeSpanStatus
	if isOrderedComparison && holdsWord {
		return nil, fmt.Errorf("%w: it does not order the built-in field %q of the %q level, so it cannot evaluate %q on it",
			tracestore.ErrFilterUnsupported, ref.Name, ref.Level, op)
	}
	value, err := builtinFieldValue(mapping.fieldType, valueExpr)
	if err != nil {
		return nil, err
	}
	if op == expression.OpNe {
		mapping.appendNegatedCondition(q, indent, " = "+mapping.placeholderSQL())
	} else {
		mapping.appendCondition(q, indent, " "+sqlComparisonOperators[op]+" "+mapping.placeholderSQL())
	}
	return append(args, value), nil
}

// builtinFieldValue reads valueExpr as the bind value a built-in field's column compares
// against. The query boundary already resolved an untyped constant against the field's type
// (RFC 0005 §5.4) before this reader ever sees it, so a mismatch here means a remote-storage
// client skipped that step, not that the caller wrote something unusual.
func builtinFieldValue(fieldType expression.FieldType, valueExpr expression.Expression) (any, error) {
	switch fieldType {
	case expression.FieldTypeString:
		v, ok := valueExpr.(*expression.StringValue)
		if !ok {
			return nil, errTypedConstant(valueExpr)
		}
		return v.Value, nil
	case expression.FieldTypeSpanKind:
		v, ok := valueExpr.(*expression.StringValue)
		if !ok {
			return nil, errTypedConstant(valueExpr)
		}
		return spanKindColumnValue(v.Value), nil
	case expression.FieldTypeSpanStatus:
		v, ok := valueExpr.(*expression.StringValue)
		if !ok {
			return nil, errTypedConstant(valueExpr)
		}
		return spanStatusColumnValue(v.Value)
	case expression.FieldTypeDuration:
		v, ok := valueExpr.(*expression.DurationValue)
		if !ok {
			return nil, errTypedConstant(valueExpr)
		}
		return v.Value.Nanoseconds(), nil
	case expression.FieldTypeTimestamp:
		v, ok := valueExpr.(*expression.TimestampValue)
		if !ok {
			return nil, errTypedConstant(valueExpr)
		}
		// The result of UnixNano is undefined for a time outside this range.
		if v.Value.Before(minUnixNanoTime) || v.Value.After(maxUnixNanoTime) {
			return nil, fmt.Errorf("%w: the timestamp %s is outside the range of signed 64-bit Unix nanoseconds",
				tracestore.ErrFilterUnsupported, v.Value.Format(time.RFC3339Nano))
		}
		return v.Value.UnixNano(), nil
	default:
		return nil, fmt.Errorf("%w: it does not support the built-in field type %q",
			tracestore.ErrFilterUnsupported, fieldType)
	}
}

// spanKindColumnValue maps an RFC 0005 span.kind word to the value the kind column stores.
// SpanKindToString (dbmodel's own writer) already lowercases every kind but the unspecified
// one, which it stores as "" rather than the word "unspecified" (jptrace.SpanKindToString);
// every other word already matches the column verbatim.
func spanKindColumnValue(word string) string {
	if word == "unspecified" {
		return ""
	}
	return word
}

// spanKindWordSQL returns the SQL that reads the kind column as an RFC 0005 span.kind word,
// the inverse of the spanKindColumnValue function.
func spanKindWordSQL(column string) string {
	return "if(" + column + " = '', 'unspecified', " + column + ")"
}

// spanStatusColumnValue maps an RFC 0005 span.status word to the value the status_code column
// stores: ptrace.StatusCode's own Stringer, capitalized, which is what the writer records
// (dbmodel.spanToRow's StatusCode field). span.status holds one of a closed set of words
// (RFC 0005 §5.4), and the query boundary already refuses anything outside it; a word this
// reader does not recognize means a remote-storage client skipped that check, so it is refused
// here too rather than silently read as "unset".
func spanStatusColumnValue(word string) (string, error) {
	switch word {
	case "ok":
		return "Ok", nil
	case "error":
		return "Error", nil
	case "unset":
		return "Unset", nil
	default:
		return "", fmt.Errorf("%w: %q is not a span.status word", tracestore.ErrFilterInvalid, word)
	}
}

// spanStatusWordSQL returns the SQL that reads the status_code column as an RFC 0005
// span.status word, the inverse of the spanStatusColumnValue function.
func spanStatusWordSQL(column string) string {
	return "lower(" + column + ")"
}

// attributeEqCandidates returns the conditions that an equality of the attribute with
// valueExpr is lowered to, against metadata already looked up. An explicitly typed constant
// (StringValue, IntValue, DoubleValue, BoolValue) matches only that stored type, the same
// authoritative-type rule RFC 0005 §5.4 states; an untyped AnyValue resolves against whatever
// type(s) metadata reports the key stored at the requested level(s), and matches nothing when
// the metadata reports no type that reads the constant.
func attributeEqCandidates(
	metadata attributeMetadata,
	key string,
	levels []expression.Level,
	valueExpr expression.Expression,
) ([]attributeCandidate, error) {
	switch v := valueExpr.(type) {
	case *expression.StringValue:
		return typedAttributeCandidates(key, levels, pcommon.ValueTypeStr, v.Value), nil
	case *expression.IntValue:
		return typedAttributeCandidates(key, levels, pcommon.ValueTypeInt, v.Value), nil
	case *expression.DoubleValue:
		return typedAttributeCandidates(key, levels, pcommon.ValueTypeDouble, v.Value), nil
	case *expression.BoolValue:
		return typedAttributeCandidates(key, levels, pcommon.ValueTypeBool, v.Value), nil
	case *expression.AnyValue:
		return untypedAttributeEqCandidates(metadata, key, levels, v.Value), nil
	default:
		return nil, errTypedConstant(valueExpr)
	}
}

// attributeLevelsToSearch returns the levels ref.Level names. An unqualified reference searches
// the span and resource levels (RFC 0005 §5.1), not all five: that wider fallback belongs to
// the legacy Attributes map's own contract, not this filter's.
func attributeLevelsToSearch(level expression.Level) ([]expression.Level, error) {
	switch level {
	case expression.LevelSpan, expression.LevelResource, expression.LevelScope, expression.LevelEvent, expression.LevelLink:
		return []expression.Level{level}, nil
	case "":
		return []expression.Level{expression.LevelSpan, expression.LevelResource}, nil
	default:
		return nil, errUnsupportedLevel(level)
	}
}

// attributeCandidate is one (level, typed value) pair an attribute comparison is lowered to: one
// condition on that level's column of that type.
type attributeCandidate struct {
	level expression.Level
	tav   typedAttributeValue
}

// typedAttributeCandidates returns one candidate per level in levels, each holding the value
// under one declared type.
func typedAttributeCandidates(
	key string,
	levels []expression.Level,
	valueType pcommon.ValueType,
	value any,
) []attributeCandidate {
	candidates := make([]attributeCandidate, 0, len(levels))
	for _, level := range levels {
		candidates = append(candidates, attributeCandidate{
			level: level,
			tav: typedAttributeValue{
				key:       key,
				value:     value,
				valueType: valueType,
			},
		})
	}
	return candidates
}

// appendAttributeCandidates ORs one condition per candidate, wrapped in parentheses so the
// disjunction survives nesting. Each condition compares the attribute with the value of its
// candidate by op. Without a candidate the condition is false.
func (q *sqlBuilder) appendAttributeCandidates(
	indent int,
	args []any,
	op expression.Operator,
	candidates []attributeCandidate,
) []any {
	q.appendNewlineAndIndent(indent)
	q.WriteString("(")
	if len(candidates) == 0 {
		q.appendNewlineAndIndent(indent + 1)
		q.WriteString("false")
	}
	for i, c := range candidates {
		if i > 0 {
			q.appendNewlineAndIndent(indent + 1)
			q.WriteString("OR")
		}
		args = q.attrBuilder.appendComparison(q, indent+1, args, c.level, op, c.tav)
	}
	q.appendNewlineAndIndent(indent)
	q.WriteString(")")
	return args
}

// collectMetadataKeys adds to keys every attribute key of predicate whose lowering reads the
// metadata. Collecting them across the whole tree avoids one attribute_metadata query per
// condition on a cold cache.
func collectMetadataKeys(predicate *expression.Call, keys pcommon.Map) {
	switch predicate.Op {
	case expression.OpAnd, expression.OpOr, expression.OpNot:
		for _, arg := range predicate.Args {
			if child, ok := arg.(*expression.Call); ok && child != nil {
				collectMetadataKeys(child, keys)
			}
		}
	case expression.OpExists, expression.OpNe, expression.OpNotIn:
		if len(predicate.Args) == 0 {
			return
		}
		if ref, ok := predicate.Args[0].(*expression.AttributeRef); ok {
			keys.PutStr(ref.Key, "")
		}
	case expression.OpEq, expression.OpIn:
		if len(predicate.Args) != 2 {
			return
		}
		ref, ok := predicate.Args[0].(*expression.AttributeRef)
		if !ok {
			return
		}
		switch v := predicate.Args[1].(type) {
		case *expression.AnyValue:
			keys.PutStr(ref.Key, "")
		case *expression.List:
			if v != nil && v.Type == "" {
				keys.PutStr(ref.Key, "")
			}
		default:
			// A typed constant names its own column and needs no metadata.
		}
	default:
		// Every other operator either has no sub-predicates to walk into or names no
		// attribute this collection pass cares about; filterCondition is what
		// refuses an operator this capability declaration does not cover.
	}
}

// untypedAttributeEqCandidates resolves an untyped constant against the metadata already looked
// up for its tree, restricted to levels rather than always searching every one metadata reports.
// A key has no candidate when no type that the metadata reports for it at the levels reads the
// constant, because no stored value can then equal it. That holds for a key that the metadata
// does not know: the metadata views record every key on insert, so only a span inserted before
// the views existed can store it, and such a span is not found.
func untypedAttributeEqCandidates(
	metadata attributeMetadata,
	key string,
	levels []expression.Level,
	raw string,
) []attributeCandidate {
	attrValue := pcommon.NewValueStr(raw)
	var candidates []attributeCandidate
	for _, level := range levels {
		for _, t := range attributeTypesForLevel(metadata[key], level) {
			tav, parseErr := parseStringToTypedValue(key, attrValue, t)
			if parseErr != nil {
				continue
			}
			candidates = append(candidates, attributeCandidate{level, tav})
		}
	}
	return candidates
}

// storedKeyFormsToSearch returns the forms in which the level stores the key, which are those
// of the types that the metadata reports for the key at the level, so that the condition reads
// only the groups of those types. A key that the metadata does not know has no form, because
// the metadata views record every key on insert; a span inserted before the views existed is
// not found. The metadata is cached, so for the lifetime of a cache entry the condition does
// not see a type that a key gained after the entry was loaded.
func storedKeyFormsToSearch(metadata attributeMetadata, key string, level expression.Level) []storedKeyForm {
	valueTypes := attributeTypesForLevel(metadata[key], level)
	var forms []storedKeyForm
	for _, form := range storedKeyForms {
		if slices.Contains(valueTypes, form.valueType) {
			forms = append(forms, form)
		}
	}
	return forms
}

// appendAttributeExists ORs, across levels, the condition that the level has an attribute with
// the key, wrapped in parentheses so the disjunction survives nesting. A level at which the
// metadata reports no type for the key is left out, and the condition is false when that
// leaves no level.
func (q *sqlBuilder) appendAttributeExists(
	indent int,
	args []any,
	metadata attributeMetadata,
	key string,
	levels []expression.Level,
) []any {
	q.appendNewlineAndIndent(indent)
	q.WriteString("(")
	isFirst := true
	for _, level := range levels {
		forms := storedKeyFormsToSearch(metadata, key, level)
		if len(forms) == 0 {
			continue
		}
		if !isFirst {
			q.appendNewlineAndIndent(indent + 1)
			q.WriteString("OR")
		}
		isFirst = false
		args = q.attrBuilder.appendExists(q, indent+1, args, level, key, forms)
	}
	if isFirst {
		q.appendNewlineAndIndent(indent + 1)
		q.WriteString("false")
	}
	q.appendNewlineAndIndent(indent)
	q.WriteString(")")
	return args
}

// appendNegatedAttributeCandidates writes the condition that the ne and not_in operators
// share: the attribute is present at one of the levels and none of the candidates holds. A bare
// NOT around the equality would also match a span that lacks the attribute, which only the
// boolean not operator may do (RFC 0005 §5.3). A candidate on a hoisted column or in the map
// layout compares the first value of a key that a span carries more than once, so the
// condition holds for such a span when only a later value is equal.
func (q *sqlBuilder) appendNegatedAttributeCandidates(
	indent int,
	args []any,
	metadata attributeMetadata,
	key string,
	levels []expression.Level,
	candidates []attributeCandidate,
) []any {
	q.appendNewlineAndIndent(indent)
	q.WriteString("(")
	args = q.appendAttributeExists(indent+1, args, metadata, key, levels)
	q.appendNewlineAndIndent(indent + 1)
	q.WriteString("AND NOT")
	args = q.appendAttributeCandidates(indent+1, args, expression.OpEq, candidates)
	q.appendNewlineAndIndent(indent)
	q.WriteString(")")
	return args
}

// buildExistsCondition lowers exists. An attribute exists when a typed group of its level
// holds the key. A built-in field exists when the span has a value of it. That is always so
// for a field of the span, the resource or the scope that cannot be absent; a field of an
// event or a link needs the span to have one.
func (q *sqlBuilder) buildExistsCondition(
	indent int,
	args []any,
	metadata attributeMetadata,
	predicate *expression.Call,
) ([]any, error) {
	if len(predicate.Args) != 1 {
		return nil, errArity(predicate)
	}
	switch ref := predicate.Args[0].(type) {
	case *expression.FieldRef:
		mapping, err := lookupBuiltinField(*ref)
		if err != nil {
			return nil, err
		}
		q.appendNewlineAndIndent(indent)
		q.WriteString(mapping.presenceSQL())
		return args, nil
	case *expression.AttributeRef:
		levels, err := attributeLevelsToSearch(ref.Level)
		if err != nil {
			return nil, err
		}
		return q.appendAttributeExists(indent, args, metadata, ref.Key, levels), nil
	default:
		return nil, fmt.Errorf("%w: %q tests a field or attribute", tracestore.ErrFilterInvalid, predicate.Op)
	}
}

// buildRegexCondition lowers regex, which searches a text value for a pattern. An attribute
// matches through its string values only, because a pattern matches text.
func (q *sqlBuilder) buildRegexCondition(indent int, args []any, predicate *expression.Call) ([]any, error) {
	if len(predicate.Args) != 2 {
		return nil, errArity(predicate)
	}
	var pattern string
	switch patternValue := predicate.Args[1].(type) {
	case *expression.StringValue:
		pattern = patternValue.Value
	case *expression.AnyValue:
		pattern = patternValue.Value
	default:
		return nil, fmt.Errorf("%w: %q takes a constant string as its pattern", tracestore.ErrFilterInvalid, predicate.Op)
	}
	// ClickHouse reports a pattern that does not parse as a server error, so the pattern is
	// refused here as an invalid filter.
	patternTree, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, fmt.Errorf("%w: %q takes a pattern in RE2 syntax: %w", tracestore.ErrFilterInvalid, predicate.Op, err)
	}
	// ClickHouse lets a dot match a newline, which RE2 and RFC 0005 §5.3 do not, so a pattern
	// with such a dot is sent with the s flag cleared, as (?-s). Any other pattern is sent as
	// written, because ClickHouse reads a pattern that starts with ^ and literal text as a
	// range of the primary key, and the flag in front of it makes the search scan the table.
	// A pattern with both, such as ^checkout-.*, pays that cost.
	if hasDotExcludingNewline(patternTree) {
		pattern = "(?-s)" + pattern
	}
	switch ref := predicate.Args[0].(type) {
	case *expression.FieldRef:
		mapping, err := lookupBuiltinField(*ref)
		if err != nil {
			return nil, err
		}
		switch mapping.fieldType {
		case expression.FieldTypeString:
		case expression.FieldTypeSpanKind:
			mapping.column = spanKindWordSQL(mapping.column)
		case expression.FieldTypeSpanStatus:
			mapping.column = spanStatusWordSQL(mapping.column)
		default:
			return nil, fmt.Errorf("%w: %q matches text, and the built-in field %q of the %q level holds a %s",
				tracestore.ErrFilterInvalid, predicate.Op, ref.Name, ref.Level, mapping.fieldType)
		}
		mapping.appendCondition(q, indent, " "+sqlComparisonOperators[expression.OpRegex]+" ?")
		return append(args, pattern), nil
	case *expression.AttributeRef:
		levels, err := attributeLevelsToSearch(ref.Level)
		if err != nil {
			return nil, err
		}
		candidates := typedAttributeCandidates(ref.Key, levels, pcommon.ValueTypeStr, pattern)
		return q.appendAttributeCandidates(indent, args, expression.OpRegex, candidates), nil
	default:
		return nil, fmt.Errorf("%w: %q matches a field or an attribute against a pattern",
			tracestore.ErrFilterInvalid, predicate.Op)
	}
}

// hasDotExcludingNewline reports whether the pattern holds a dot that RE2 does not let match
// a newline.
func hasDotExcludingNewline(patternTree *syntax.Regexp) bool {
	if patternTree.Op == syntax.OpAnyCharNotNL {
		return true
	}
	return slices.ContainsFunc(patternTree.Sub, hasDotExcludingNewline)
}

// buildMembershipCondition lowers the in and not_in operators. On an attribute, the in
// operator is the disjunction of the equalities with each element, so that it matches whatever
// the eq operator matches (RFC 0005 §5.4), and the not_in operator follows the rule of ne.
func (q *sqlBuilder) buildMembershipCondition(
	indent int,
	args []any,
	metadata attributeMetadata,
	predicate *expression.Call,
) ([]any, error) {
	if len(predicate.Args) != 2 {
		return nil, errArity(predicate)
	}
	list, ok := predicate.Args[1].(*expression.List)
	if !ok || list == nil || len(list.Values) == 0 {
		return nil, fmt.Errorf("%w: %q takes a list with at least one element", tracestore.ErrFilterInvalid, predicate.Op)
	}
	switch ref := predicate.Args[0].(type) {
	case *expression.FieldRef:
		return q.buildFieldMembership(indent, args, predicate.Op, *ref, list)
	case *expression.AttributeRef:
		constants := make([]expression.Expression, 0, len(list.Values))
		for _, element := range list.Values {
			constant, err := tracestore.ReadFilterElement(list, "", element)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
			}
			constants = append(constants, constant)
		}
		return q.buildAttributeMembership(indent, args, metadata, *ref, constants, predicate.Op == expression.OpNotIn)
	default:
		return nil, fmt.Errorf("%w: %q tests a field or attribute against a list", tracestore.ErrFilterInvalid, predicate.Op)
	}
}

// buildAttributeMembership lowers the test that the attribute equals one of the constants,
// which is the eq operator for one constant and the in operator for a list. When isNegated is
// set it lowers the ne or the not_in operator.
func (q *sqlBuilder) buildAttributeMembership(
	indent int,
	args []any,
	metadata attributeMetadata,
	ref expression.AttributeRef,
	constants []expression.Expression,
	isNegated bool,
) ([]any, error) {
	levels, err := attributeLevelsToSearch(ref.Level)
	if err != nil {
		return nil, err
	}
	var candidates []attributeCandidate
	for _, constant := range constants {
		constantCandidates, err := attributeEqCandidates(metadata, ref.Key, levels, constant)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, constantCandidates...)
	}
	if isNegated {
		return q.appendNegatedAttributeCandidates(indent, args, metadata, ref.Key, levels, candidates), nil
	}
	return q.appendAttributeCandidates(indent, args, expression.OpEq, candidates), nil
}

// buildFieldMembership lowers in and not_in on a built-in field onto its column, with each
// element read as the type of the field.
func (q *sqlBuilder) buildFieldMembership(
	indent int,
	args []any,
	op expression.Operator,
	ref expression.FieldRef,
	list *expression.List,
) ([]any, error) {
	mapping, err := lookupBuiltinField(ref)
	if err != nil {
		return nil, err
	}
	for _, element := range list.Values {
		constant, err := tracestore.ReadFilterElement(list, mapping.fieldType, element)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
		}
		value, err := builtinFieldValue(mapping.fieldType, constant)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}
	placeholderSQL := mapping.placeholderSQL()
	elementsSQL := "(" + placeholderSQL + strings.Repeat(", "+placeholderSQL, len(list.Values)-1) + ")"
	if op == expression.OpNotIn {
		mapping.appendNegatedCondition(q, indent, " IN "+elementsSQL)
	} else {
		mapping.appendCondition(q, indent, " IN "+elementsSQL)
	}
	return args, nil
}

func attributeTypesForLevel(lt attrTypes, level expression.Level) []pcommon.ValueType {
	switch level {
	case expression.LevelResource:
		return lt.resource
	case expression.LevelScope:
		return lt.scope
	case expression.LevelEvent:
		return lt.event
	case expression.LevelLink:
		return lt.link
	default: // expression.LevelSpan
		return lt.span
	}
}

func errArity(predicate *expression.Call) error {
	return fmt.Errorf("%w: %q cannot take %d arguments", tracestore.ErrFilterInvalid, predicate.Op, len(predicate.Args))
}

func errUnsupportedLevel(level expression.Level) error {
	return fmt.Errorf("%w: it does not index the %q level", tracestore.ErrFilterUnsupported, level)
}

func errUnsupportedField(ref expression.FieldRef) error {
	return fmt.Errorf("%w: it does not support the built-in field %q of the %q level",
		tracestore.ErrFilterUnsupported, ref.Name, ref.Level)
}

// errTypedConstant refuses a constant whose Go type this lowering does not handle for the
// position it appeared in: an explicitly typed constant of the wrong kind for the field or
// attribute it compares, or a constant type this backend does not lower at all yet.
func errTypedConstant(value expression.Expression) error {
	return fmt.Errorf("%w: %s declares a type this comparison cannot use",
		tracestore.ErrFilterUnsupported, constantKind(value))
}

func constantKind(value expression.Expression) string {
	switch value.(type) {
	case *expression.StringValue:
		return "a string constant"
	case *expression.IntValue:
		return "an integer constant"
	case *expression.DoubleValue:
		return "a floating-point constant"
	case *expression.BoolValue:
		return "a boolean constant"
	case *expression.DurationValue:
		return "a duration constant"
	case *expression.TimestampValue:
		return "a timestamp constant"
	default:
		return "that operand"
	}
}
