// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// FilterCapabilities declares the part of the RFC 0005 filter model this reader evaluates
// natively: the boolean combinators, equality over every level's attributes, a handful of
// built-in fields whose columns already exist (span.name, span.kind, span.status, span.duration,
// resource.service), and the four ordered comparisons on span.duration. The declaration is
// coarser than the lowering, as the elasticsearch backend's is: an ordered comparison on an
// attribute or on another built-in field is refused in the lowering, not here. ne, regex,
// exists, in/not_in and the remaining built-in fields are follow-up work; a filter naming any
// of them is refused rather than approximated.
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
			expression.OpGt,
			expression.OpLt,
			expression.OpGte,
			expression.OpLte,
		},
	}
}

// sqlComparisonOperators maps each comparison operator this lowering evaluates to its SQL
// spelling. Only span.duration takes the four ordered ones.
var sqlComparisonOperators = map[expression.Operator]string{
	expression.OpEq:  "=",
	expression.OpGt:  ">",
	expression.OpLt:  "<",
	expression.OpGte: ">=",
	expression.OpLte: "<=",
}

// builtinFieldColumn is one built-in field's SQL column and the type its constant is read as.
type builtinFieldColumn struct {
	column    string
	fieldType expression.FieldType
}

// builtinFieldColumns is the built-in fields this lowering compares, keyed by level and name.
// The spans table stores most of the others (trace_state, parent_span_id, status_message,
// scope_name, scope_version, events.name, links.trace_id and so on), but this first increment
// maps only these five; a field not listed here is refused (errUnsupportedField) rather than
// approximated. resource.schemaURL and scope.schemaURL have no column at all.
var builtinFieldColumns = map[expression.Level]map[string]builtinFieldColumn{
	expression.LevelSpan: {
		expression.SpanFieldName: {
			column:    "s.name",
			fieldType: expression.FieldTypeString,
		},
		expression.SpanFieldKind: {
			column:    "s.kind",
			fieldType: expression.FieldTypeSpanKind,
		},
		expression.SpanFieldStatus: {
			column:    "s.status_code",
			fieldType: expression.FieldTypeSpanStatus,
		},
		expression.SpanFieldDuration: {
			column:    "s.duration",
			fieldType: expression.FieldTypeDuration,
		},
	},
	expression.LevelResource: {
		expression.ResourceFieldService: {
			column:    "s.service_name",
			fieldType: expression.FieldTypeString,
		},
	},
}

// lookupUntypedMetadata resolves every untyped attribute key in predicate against
// attribute_metadata in a single query. Keys with no metadata come back absent, which the
// lowering reads as a literal string match, so a key is never looked up twice.
func (r *Reader) lookupUntypedMetadata(ctx context.Context, predicate *expression.Call) (attributeMetadata, error) {
	keys := pcommon.NewMap()
	collectUntypedAttributeKeys(predicate, keys)
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
// returns the updated bind arguments. metadata is the untyped attribute metadata already looked
// up for the whole tree by lookupUntypedMetadata, so no part of the lowering queries
// attribute_metadata again. It mirrors the elasticsearch backend's buildFilterQuery: the filter
// arrives already checked against FilterCapabilities when it comes through the query service,
// but a remote-storage client can reach this reader without that check, so every refusal is
// made here too rather than assumed.
func buildFilterCondition(
	q *strings.Builder,
	indent int,
	args []any,
	metadata attributeMetadata,
	predicate *expression.Call,
) ([]any, error) {
	switch predicate.Op {
	case expression.OpAnd, expression.OpOr:
		return buildBooleanCondition(q, indent, args, metadata, predicate)
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
		appendNewlineAndIndent(q, indent)
		q.WriteString("NOT (")
		args, err := buildFilterCondition(q, indent+1, args, metadata, child)
		if err != nil {
			return nil, err
		}
		appendNewlineAndIndent(q, indent)
		q.WriteString(")")
		return args, nil
	case expression.OpEq, expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte:
		return buildComparisonCondition(q, indent, args, metadata, predicate)
	default:
		return nil, fmt.Errorf("%w: it does not support the operator %q", tracestore.ErrFilterUnsupported, predicate.Op)
	}
}

// buildBooleanCondition lowers and/or: each argument is itself a predicate, joined by the
// operator's SQL keyword and wrapped in parentheses so precedence survives nesting.
func buildBooleanCondition(
	q *strings.Builder,
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
	appendNewlineAndIndent(q, indent)
	q.WriteString("(")
	for i, arg := range predicate.Args {
		child, ok := arg.(*expression.Call)
		if !ok || child == nil {
			return nil, fmt.Errorf("%w: %q combines predicates, not values", tracestore.ErrFilterInvalid, predicate.Op)
		}
		if i > 0 {
			appendNewlineAndIndent(q, indent+1)
			q.WriteString(sep)
		}
		var err error
		args, err = buildFilterCondition(q, indent+1, args, metadata, child)
		if err != nil {
			return nil, err
		}
	}
	appendNewlineAndIndent(q, indent)
	q.WriteString(")")
	return args, nil
}

// buildComparisonCondition dispatches a comparison to the built-in-field or attribute path by
// the shape of its first operand; a finalized filter puts the reference first (RFC 0005 §5.3)
// and a constant second.
func buildComparisonCondition(
	q *strings.Builder,
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
		return buildFieldComparison(q, indent, args, predicate.Op, *ref, predicate.Args[1])
	case *expression.AttributeRef:
		if predicate.Op != expression.OpEq {
			return nil, fmt.Errorf("%w: it does not order attributes, so it cannot evaluate %q on %q",
				tracestore.ErrFilterUnsupported, predicate.Op, ref.Key)
		}
		return buildAttributeComparison(q, indent, args, metadata, *ref, predicate.Args[1])
	default:
		return nil, fmt.Errorf("%w: %q compares a field or attribute against a constant",
			tracestore.ErrFilterInvalid, predicate.Op)
	}
}

// buildFieldComparison lowers a comparison of a built-in field onto its column. Equality
// reaches every mapped field; the ordered comparisons reach span.duration only, since the other
// mapped fields hold words (span.kind, span.status, which RFC 0005 §5.4 says have no order) or
// names, whose lexicographic ordering this increment does not lower.
func buildFieldComparison(
	q *strings.Builder,
	indent int,
	args []any,
	op expression.Operator,
	ref expression.FieldRef,
	valueExpr expression.Expression,
) ([]any, error) {
	// Every level is indexed, so a built-in field with no entry is unsupported as a field,
	// whichever level it belongs to.
	mapping, ok := builtinFieldColumns[ref.Level][ref.Name]
	if !ok {
		return nil, errUnsupportedField(ref)
	}
	if op != expression.OpEq && mapping.fieldType != expression.FieldTypeDuration {
		return nil, fmt.Errorf("%w: it does not order the built-in field %q of the %q level, so it cannot evaluate %q on it",
			tracestore.ErrFilterUnsupported, ref.Name, ref.Level, op)
	}
	value, err := builtinFieldValue(mapping.fieldType, valueExpr)
	if err != nil {
		return nil, err
	}
	appendNewlineAndIndent(q, indent)
	q.WriteString(mapping.column + " " + sqlComparisonOperators[op] + " ?")
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

// buildAttributeComparison lowers an attribute equality against metadata already looked up.
// An explicitly typed constant (StringValue, IntValue, DoubleValue, BoolValue) matches only that
// stored type, the same authoritative-type rule RFC 0005 §5.4 states; an untyped AnyValue
// resolves against whatever type(s) metadata reports the key stored at the requested level(s),
// falling back to a literal string match when metadata knows nothing about it.
func buildAttributeComparison(
	q *strings.Builder,
	indent int,
	args []any,
	metadata attributeMetadata,
	ref expression.AttributeRef,
	valueExpr expression.Expression,
) ([]any, error) {
	levels, err := attributeLevelsToSearch(ref.Level)
	if err != nil {
		return nil, err
	}
	switch v := valueExpr.(type) {
	case *expression.StringValue:
		return buildAttributeEqAcrossLevels(q, indent, args, ref.Key, levels, pcommon.ValueTypeStr, v.Value), nil
	case *expression.IntValue:
		return buildAttributeEqAcrossLevels(q, indent, args, ref.Key, levels, pcommon.ValueTypeInt, v.Value), nil
	case *expression.DoubleValue:
		return buildAttributeEqAcrossLevels(q, indent, args, ref.Key, levels, pcommon.ValueTypeDouble, v.Value), nil
	case *expression.BoolValue:
		return buildAttributeEqAcrossLevels(q, indent, args, ref.Key, levels, pcommon.ValueTypeBool, v.Value), nil
	case *expression.AnyValue:
		return buildUntypedAttributeEq(metadata, q, indent, args, ref.Key, levels, v.Value)
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

// attributeLocation is where a level's attributes live: the arrayExists column prefix
// query_builder.go's helpers already use, and whether that level is a nested array (event,
// link) rather than a flat one (span, resource, scope).
func attributeLocation(level expression.Level) (prefix string, nested bool) {
	switch level {
	case expression.LevelResource:
		return "resource", false
	case expression.LevelScope:
		return "scope", false
	case expression.LevelEvent:
		return "events", true
	case expression.LevelLink:
		return "links", true
	default: // expression.LevelSpan
		return "", false
	}
}

// attributeCandidate is one (level, typed value) pair an attribute equality is lowered to: one
// arrayExists over that level's column of that type.
type attributeCandidate struct {
	level expression.Level
	tav   typedAttributeValue
}

// buildAttributeEqAcrossLevels ORs an equality of one declared type across every level in levels.
func buildAttributeEqAcrossLevels(
	q *strings.Builder,
	indent int,
	args []any,
	key string,
	levels []expression.Level,
	valueType pcommon.ValueType,
	value any,
) []any {
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
	return appendAttributeCandidates(q, indent, args, candidates)
}

// appendAttributeCandidates ORs one arrayExists (or nested arrayExists) per candidate, wrapped
// in parentheses so the disjunction survives nesting.
func appendAttributeCandidates(q *strings.Builder, indent int, args []any, candidates []attributeCandidate) []any {
	appendNewlineAndIndent(q, indent)
	q.WriteString("(")
	for i, c := range candidates {
		if i > 0 {
			appendNewlineAndIndent(q, indent+1)
			q.WriteString("OR")
		}
		prefix, nested := attributeLocation(c.level)
		if nested {
			appendNestedArrayExists(q, indent+1, prefix, c.tav.valueType)
		} else {
			appendArrayExists(q, indent+1, prefix, c.tav.valueType)
		}
		args = append(args, c.tav.key, c.tav.value)
	}
	appendNewlineAndIndent(q, indent)
	q.WriteString(")")
	return args
}

// collectUntypedAttributeKeys walks predicate and adds to keys every key that an untyped
// (AnyValue) attribute equality names, so lookupUntypedMetadata can fetch the metadata for the
// whole tree in one query. Without this, a filter with several untyped attribute predicates
// would pay one attribute_metadata query per predicate on a cold cache, where the legacy
// Attributes path already batches every key into one.
func collectUntypedAttributeKeys(predicate *expression.Call, keys pcommon.Map) {
	switch predicate.Op {
	case expression.OpAnd, expression.OpOr, expression.OpNot:
		for _, arg := range predicate.Args {
			if child, ok := arg.(*expression.Call); ok && child != nil {
				collectUntypedAttributeKeys(child, keys)
			}
		}
	case expression.OpEq:
		if len(predicate.Args) != 2 {
			return
		}
		ref, ok := predicate.Args[0].(*expression.AttributeRef)
		if !ok {
			return
		}
		if _, ok := predicate.Args[1].(*expression.AnyValue); ok {
			keys.PutStr(ref.Key, "")
		}
	default:
		// Every other operator either has no sub-predicates to walk into or names no
		// attribute this collection pass cares about; buildFilterCondition is what
		// refuses an operator this capability declaration does not cover.
	}
}

// buildUntypedAttributeEq resolves an untyped constant against the metadata already looked up for
// its tree, restricted to levels rather than always searching every one metadata reports. A key
// the metadata does not know resolves to nothing here and takes the literal string fallback.
func buildUntypedAttributeEq(
	metadata attributeMetadata,
	q *strings.Builder,
	indent int,
	args []any,
	key string,
	levels []expression.Level,
	raw string,
) ([]any, error) {
	attrValue := pcommon.NewValueStr(raw)
	levelTypes := metadata[key]

	var candidates []attributeCandidate
	for _, level := range levels {
		for _, t := range attributeTypesForLevel(levelTypes, level) {
			tav, parseErr := parseStringToTypedValue(key, attrValue, t)
			if parseErr != nil {
				continue
			}
			candidates = append(candidates, attributeCandidate{level, tav})
		}
	}
	if len(candidates) == 0 {
		// No metadata for this key at any requested level: fall back to a literal string
		// match, the same default appendStringAttributeFallback uses for the legacy map.
		return buildAttributeEqAcrossLevels(q, indent, args, key, levels, pcommon.ValueTypeStr, raw), nil
	}

	return appendAttributeCandidates(q, indent, args, candidates), nil
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
