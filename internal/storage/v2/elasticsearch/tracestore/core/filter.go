// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"strconv"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	esquery "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/query"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// eventNameKey is the logs.fields key the write path stores a span event's name under
// (spanEventsToDbSpanLogs), which is what makes the event.name built-in field readable.
const eventNameKey = "event"

// eventNameAsAttribute reads event.name as the logs.fields entry it is stored in, so the
// built-in field and an event attribute share one lowering.
var eventNameAsAttribute = reference{
	name:      eventNameKey,
	level:     expression.LevelEvent,
	attribute: true,
}

// reference is what a lowering needs to know about a reference term: the level it names, the key or
// field name under it, and whether it is an attribute. The AST has a distinct type per kind
// (RFC 0005 §5.1); reading one into this shape once keeps every lowering below from switching on
// the term type again.
type reference struct {
	name      string
	level     expression.Level
	attribute bool
}

// asReference reads a term that names a value on the span. A collection reference is not one: it is
// only ever the first argument of `some`, which this reader does not serve.
func asReference(term expression.Expression) (reference, bool) {
	switch ref := term.(type) {
	case *expression.AttributeRef:
		if ref != nil {
			return reference{name: ref.Key, level: ref.Level, attribute: true}, true
		}
	case *expression.FieldRef:
		if ref != nil {
			return reference{name: ref.Name, level: ref.Level}, true
		}
	default:
	}
	return reference{}, false
}

// isField reports whether the reference names one particular built-in field.
func (r reference) isField(level expression.Level, name string) bool {
	return !r.attribute && r.level == level && r.name == name
}

// attributeLocations is where a level's attributes live in a span document: the flattened
// object fields, whose leaf is the attribute key, and the nested key/value arrays. Both are
// searched because which of the two the write path produced depends on the tags-as-fields
// setting in force when the span was indexed, and that setting can change over the life of
// an index. Instrumentation-scope attributes are folded into the span's own tags and link
// attributes are not indexed at all, so neither level appears here (RFC 0005 §1.6).
var attributeLocations = map[expression.Level]attributeLocation{
	expression.LevelSpan: {
		object: []string{objectTagsField},
		nested: []string{nestedTagsField},
	},
	expression.LevelResource: {
		object: []string{objectProcessTagsField},
		nested: []string{nestedProcessTagsField},
	},
	expression.LevelEvent: {
		nested: []string{nestedLogFieldsField},
	},
	// An unqualified reference searches the span and resource levels (RFC 0005 §5.1). It
	// deliberately stops short of the event level that the legacy Tags search also covers:
	// the legacy field keeps its behavior, and the filter follows the documented contract.
	"": {
		object: objectTagFieldList,
		nested: []string{nestedTagsField, nestedProcessTagsField},
	},
}

type attributeLocation struct {
	object []string
	nested []string
}

// valueMatch builds the query matching a value held in one Elasticsearch field. An
// attribute lives in several fields at once, so the comparison is chosen from the operator
// once and then applied to each of them.
type valueMatch func(field string) esquery.Query

// FilterCapabilities declares the part of the RFC 0005 filter model this reader evaluates.
// It omits the scope and link levels, which the schema does not index separately,
// and the `some` quantifier, whose correlated matching over a span's events is not
// implemented yet. Which built-in fields are served is not declarable — a field name is
// indistinguishable from an attribute key — so buildFilterQuery refuses the ones this
// schema has no field for.
func FilterCapabilities() tracestore.FilterCapabilities {
	return tracestore.FilterCapabilities{
		Levels: []expression.Level{
			expression.LevelSpan,
			expression.LevelResource,
			expression.LevelEvent,
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

// buildFilterQuery lowers a structured query filter (RFC 0005) into an Elasticsearch query.
// The boolean combinators become the bool query's must / should / must_not clauses, and a
// leaf comparison becomes a term, regexp, or range query over the fields the referenced
// value lives in. A predicate this schema cannot answer is refused rather than approximated,
// so a caller never reads a narrower answer as the whole one.
//
// The filter arrives already checked against FilterCapabilities when it comes through the
// query service, but a remote-storage client can reach this reader without that check, so
// every refusal is made here too rather than assumed.
func (s *SpanReader) buildFilterQuery(predicate *expression.Call) (esquery.Query, error) {
	switch predicate.Op {
	case expression.OpAnd, expression.OpOr:
		args, err := s.buildCombinedArgs(predicate)
		if err != nil {
			return nil, err
		}
		if predicate.Op == expression.OpAnd {
			return allOf(args), nil
		}
		return anyOf(args), nil

	case expression.OpNot:
		if len(predicate.Args) != 1 {
			return nil, errArity(predicate)
		}
		args, err := s.buildCombinedArgs(predicate)
		if err != nil {
			return nil, err
		}
		return esquery.NewBoolQuery().MustNot(args[0]), nil

	case expression.OpEq, expression.OpRegex,
		expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte:
		ref, value, err := refAndConstantArgs(predicate)
		if err != nil {
			return nil, err
		}
		return s.buildComparison(predicate.Op, ref, value)

	case expression.OpNe:
		ref, value, err := refAndConstantArgs(predicate)
		if err != nil {
			return nil, err
		}
		present, err := s.buildExists(ref)
		if err != nil {
			return nil, err
		}
		equal, err := s.buildComparison(expression.OpEq, ref, value)
		if err != nil {
			return nil, err
		}
		return holdsSomethingElse(present, equal), nil

	case expression.OpIn, expression.OpNotIn:
		return s.buildMembership(predicate)

	case expression.OpExists:
		ref, err := refArg(predicate)
		if err != nil {
			return nil, err
		}
		return s.buildExists(ref)

	default:
		return nil, fmt.Errorf("%w: it does not support the operator %q",
			tracestore.ErrFilterUnsupported, predicate.Op)
	}
}

// buildCombinedArgs lowers the arguments of a boolean combinator, each of which is itself a
// predicate rather than a value.
func (s *SpanReader) buildCombinedArgs(predicate *expression.Call) ([]esquery.Query, error) {
	if len(predicate.Args) == 0 {
		return nil, errArity(predicate)
	}
	queries := make([]esquery.Query, 0, len(predicate.Args))
	for _, arg := range predicate.Args {
		call, ok := arg.(*expression.Call)
		if !ok {
			return nil, fmt.Errorf("%w: %q combines predicates, not values",
				tracestore.ErrFilterInvalid, predicate.Op)
		}
		query, err := s.buildFilterQuery(call)
		if err != nil {
			return nil, err
		}
		queries = append(queries, query)
	}
	return queries, nil
}

// buildMembership lowers `in` and `not_in` as the disjunction of equalities they stand for,
// so membership reaches every reference an equality does without a second lowering.
func (s *SpanReader) buildMembership(predicate *expression.Call) (esquery.Query, error) {
	ref, list, err := refAndListArgs(predicate)
	if err != nil {
		return nil, err
	}
	if len(list.Values) == 0 {
		return nil, fmt.Errorf("%w: %q compares against an empty list",
			tracestore.ErrFilterInvalid, predicate.Op)
	}
	// The presence test is built even for `in`, which does not need it, so that a reference
	// this reader cannot read is refused before the members are lowered one by one.
	present, err := s.buildExists(ref)
	if err != nil {
		return nil, err
	}
	// ReadFilterElement needs the field's type to read an element the list left untyped. An
	// attribute has no type to pass, and the element then comes back untyped, which is the form
	// this schema searches.
	var fieldType expression.FieldType
	if !ref.attribute {
		if field, found := expression.LookupField(ref.level, ref.name); found {
			fieldType = field.Type
		}
	}
	members := make([]esquery.Query, 0, len(list.Values))
	for _, value := range list.Values {
		element, err := tracestore.ReadFilterElement(list, fieldType, value)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", tracestore.ErrFilterInvalid, err)
		}
		member, err := s.buildComparison(expression.OpEq, ref, element)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	if predicate.Op == expression.OpIn {
		return anyOf(members), nil
	}
	return holdsSomethingElse(present, anyOf(members)), nil
}

// holdsSomethingElse builds the negated leaf comparisons, `ne` and `not_in`. A comparison
// against a value the span does not carry is false, and only a boolean `not` flips that
// (RFC 0005 §5.3), so these ask for the spans that hold the reference and hold something
// else — not merely the ones a bare must_not would leave, which include every span missing
// the reference entirely.
func holdsSomethingElse(present, matching esquery.Query) esquery.Query {
	return esquery.NewBoolQuery().Must(present).MustNot(matching)
}

func (s *SpanReader) buildComparison(
	op expression.Operator,
	ref reference,
	value expression.Expression,
) (esquery.Query, error) {
	if ref.isField(expression.LevelSpan, expression.SpanFieldDuration) {
		return buildDurationComparison(op, value)
	}
	text, err := constantText(value)
	if err != nil {
		return nil, err
	}
	if ref.attribute {
		return s.buildAttributeComparison(op, ref, text)
	}
	switch {
	case ref.isField(expression.LevelSpan, expression.SpanFieldName):
		return buildTextComparison(operationNameField, op, ref, text)
	case ref.isField(expression.LevelResource, expression.ResourceFieldService):
		return buildTextComparison(serviceNameField, op, ref, text)
	case ref.isField(expression.LevelEvent, expression.EventFieldName):
		return s.buildAttributeComparison(op, eventNameAsAttribute, text)
	default:
		return nil, errUnsupportedField(ref)
	}
}

// constantText returns the text that a constant contributes to a comparison. It accepts an untyped
// constant and one declaring string, because this schema compares text whether the reference is an
// attribute or a built-in field holding text, so a string declaration asks for the comparison the
// caller already gets. Any other declared type names typed storage this schema lacks
// (RFC 0005 §5.4).
func constantText(value expression.Expression) (string, error) {
	switch constant := value.(type) {
	case *expression.AnyValue:
		if constant != nil {
			return constant.Value, nil
		}
	case *expression.StringValue:
		if constant != nil {
			return constant.Value, nil
		}
	default:
	}
	return "", errTypedConstant(value)
}

// lengthOfTime reads the duration a constant carries. A finalized filter compares the duration field
// against a duration node; an untyped constant arrives from a tree that reached this reader without
// being finalized, and is read the way finalizing would have read it.
func lengthOfTime(value expression.Expression) (time.Duration, error) {
	switch constant := value.(type) {
	case *expression.DurationValue:
		if constant != nil {
			return constant.Value, nil
		}
	case *expression.AnyValue:
		if constant == nil {
			break
		}
		// An untyped constant reaches here from a tree that was not finalized, so it is read the way
		// finalizing would have read it rather than by a second parser of this package's own.
		read, err := tracestore.ReadFilterConstant(expression.FieldTypeDuration, constant.Value)
		if err != nil {
			return 0, fmt.Errorf(`%w: %q is not a duration such as "2s": %w`,
				tracestore.ErrFilterInvalid, constant.Value, err)
		}
		if duration, ok := read.(*expression.DurationValue); ok {
			return duration.Value, nil
		}
	default:
	}
	return 0, errTypedConstant(value)
}

func (s *SpanReader) buildExists(ref reference) (esquery.Query, error) {
	switch {
	case ref.attribute:
		return s.buildAttributeExists(ref)
	case ref.isField(expression.LevelSpan, expression.SpanFieldName):
		return esquery.NewExistsQuery(operationNameField), nil
	case ref.isField(expression.LevelResource, expression.ResourceFieldService):
		return esquery.NewExistsQuery(serviceNameField), nil
	case ref.isField(expression.LevelSpan, expression.SpanFieldDuration):
		return esquery.NewExistsQuery(durationField), nil
	case ref.isField(expression.LevelEvent, expression.EventFieldName):
		return s.buildAttributeExists(eventNameAsAttribute)
	default:
		return nil, errUnsupportedField(ref)
	}
}

func (s *SpanReader) buildAttributeComparison(
	op expression.Operator,
	ref reference,
	value string,
) (esquery.Query, error) {
	locations, ok := attributeLocations[ref.level]
	if !ok {
		return nil, errUnsupportedLevel(ref.level)
	}
	if isError, ok := asErrorTagEquality(op, ref, value); ok {
		// The write path records the error tag only for a span whose status is an error
		// (getTagFromStatusCode), so a span that succeeded carries no error tag and matching
		// error=false literally returns nothing. Read it as the complement — every span that
		// is not an error — as the legacy tag search and the in-memory store both do (#9096).
		errored := s.attributeQuery(locations, ref.name, termMatch("true"))
		if isError {
			return errored, nil
		}
		return esquery.NewBoolQuery().MustNot(errored), nil
	}
	match, err := attributeValueMatch(op, ref, value)
	if err != nil {
		return nil, err
	}
	return s.attributeQuery(locations, ref.name, match), nil
}

// attributeQuery matches an attribute in every field its level keeps attributes in.
func (s *SpanReader) attributeQuery(locations attributeLocation, key string, match valueMatch) esquery.Query {
	queries := make([]esquery.Query, 0, len(locations.object)+len(locations.nested))
	for _, field := range locations.object {
		queries = append(queries, match(s.objectAttributeField(field, key)))
	}
	for _, path := range locations.nested {
		queries = append(queries, esquery.NewNestedQuery(path, esquery.NewBoolQuery().Must(
			esquery.NewTermQuery(nestedField(path, tagKeyField), key),
			match(nestedField(path, tagValueField)),
		)))
	}
	return anyOf(queries)
}

func (s *SpanReader) buildAttributeExists(ref reference) (esquery.Query, error) {
	locations, ok := attributeLocations[ref.level]
	if !ok {
		return nil, errUnsupportedLevel(ref.level)
	}
	queries := make([]esquery.Query, 0, len(locations.object)+len(locations.nested))
	for _, field := range locations.object {
		queries = append(queries, esquery.NewExistsQuery(s.objectAttributeField(field, ref.name)))
	}
	for _, path := range locations.nested {
		queries = append(queries, esquery.NewNestedQuery(path,
			esquery.NewTermQuery(nestedField(path, tagKeyField), ref.name)))
	}
	return anyOf(queries), nil
}

// objectAttributeField is where the flattened representation keeps one attribute. Its leaf
// is the attribute key with dots replaced, because a dot in a field name would read as a
// step into a subobject.
func (s *SpanReader) objectAttributeField(field, key string) string {
	return field + "." + s.dotReplacer.ReplaceDot(key)
}

func nestedField(path, field string) string {
	return path + "." + field
}

// attributeValueMatch chooses how a comparison tests an attribute value. Attribute values
// are indexed as keywords, so equality and patterns work and ordering does not.
func attributeValueMatch(op expression.Operator, ref reference, value string) (valueMatch, error) {
	switch op {
	case expression.OpEq:
		return termMatch(value), nil
	case expression.OpRegex:
		return forThisEngine(value)
	default:
		return nil, errUnorderedValue(op, ref)
	}
}

// forThisEngine builds the match for this engine's regexp query, or refuses a pattern it would read
// differently than the query boundary said it means (RFC 0005 §5.3).
//
// Three differences. The query matches a whole indexed term while a filter's pattern matches anywhere
// in the value, so the pattern is wrapped: soundly, because the boundary refuses a pattern that
// anchors itself, and the group keeps a top-level alternation from swallowing the wildcards.
//
// This dialect also has no Perl shorthands. It reads `\d` as the letter d rather than as a digit, so a
// pattern carrying one is refused rather than sent to match something else; the same pattern written
// `[0-9]` is served. The refusal is lexical because it has to be: parsing turns `\d` and `[0-9]` into
// one character class, and there is no telling them apart afterwards.
//
// And it reads &, @, ~, <n-m>, # and <identifier> as operators unless the query opts out, so the flags
// are pinned to NONE. Without that an address like `@example.com` is an operator rather than the text
// the filter asked for, and the capability is lost instead of narrowed.
func forThisEngine(pattern string) (valueMatch, error) {
	if escape, ok := perlShorthand(pattern); ok {
		return nil, fmt.Errorf(
			`%w: it reads %q as the literal character, so write the class out ("[0-9]" for "\d")`,
			tracestore.ErrFilterUnsupported, escape,
		)
	}
	wrapped := ".*(" + pattern + ").*"
	return func(field string) esquery.Query {
		return esquery.NewRegexpQuery(field, wrapped).Flags("NONE")
	}, nil
}

// perlShorthand finds the first backslash escape of a letter or a digit, which is what a Perl
// shorthand looks like. An escaped punctuation character — `\.`, `\[` — means the same thing in both
// dialects, so those pass.
func perlShorthand(pattern string) (string, bool) {
	for i := 0; i < len(pattern)-1; i++ {
		if pattern[i] != '\\' {
			continue
		}
		next := pattern[i+1]
		if next == '\\' {
			i++ // An escaped backslash is a literal one, and does not escape what follows it.
			continue
		}
		if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || (next >= '0' && next <= '9') {
			return pattern[i : i+2], true
		}
	}
	return "", false
}

func termMatch(value string) valueMatch {
	return func(field string) esquery.Query { return esquery.NewTermQuery(field, value) }
}

// buildTextComparison compares a built-in field held as a keyword — an operation name or a
// service name — which supports equality and patterns but carries no order worth exposing.
func buildTextComparison(
	field string,
	op expression.Operator,
	ref reference,
	value string,
) (esquery.Query, error) {
	switch op {
	case expression.OpEq:
		return esquery.NewTermQuery(field, value), nil
	case expression.OpRegex:
		match, err := forThisEngine(value)
		if err != nil {
			return nil, err
		}
		return match(field), nil
	default:
		return nil, errUnorderedValue(op, ref)
	}
}

// durationComparisons is how each operator tests the duration field, which is the one
// ordered value this schema indexes numerically.
var durationComparisons = map[expression.Operator]func(micros uint64) esquery.Query{
	expression.OpEq: func(micros uint64) esquery.Query {
		return esquery.NewTermQuery(durationField, micros)
	},
	expression.OpGt: func(micros uint64) esquery.Query {
		return esquery.NewRangeQuery(durationField).Gt(micros)
	},
	expression.OpGte: func(micros uint64) esquery.Query {
		return esquery.NewRangeQuery(durationField).Gte(micros)
	},
	expression.OpLt: func(micros uint64) esquery.Query {
		return esquery.NewRangeQuery(durationField).Lt(micros)
	},
	expression.OpLte: func(micros uint64) esquery.Query {
		return esquery.NewRangeQuery(durationField).Lte(micros)
	},
}

// buildDurationComparison compares the span duration, which the field holds in microseconds. The
// operator is resolved before the value is read, so an operator the duration has no answer for is
// refused as that rather than as a value of the wrong kind.
func buildDurationComparison(op expression.Operator, value expression.Expression) (esquery.Query, error) {
	compare, ok := durationComparisons[op]
	if !ok {
		return nil, fmt.Errorf("%w: it does not support the operator %q on a duration",
			tracestore.ErrFilterUnsupported, op)
	}
	duration, err := lengthOfTime(value)
	if err != nil {
		return nil, err
	}
	return compare(model.DurationAsMicroseconds(duration)), nil
}

// asErrorTagEquality reports through ok whether a predicate tests the error tag for a
// boolean, the one attribute whose absence carries meaning, and through isError which side of
// it was asked for. The tag is written on the span, so a resource-level reference is not it.
func asErrorTagEquality(
	op expression.Operator,
	ref reference,
	value string,
) (isError bool, ok bool) {
	if op != expression.OpEq || ref.name != errorTag {
		return false, false
	}
	if ref.level != "" && ref.level != expression.LevelSpan {
		return false, false
	}
	isError, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}
	return isError, true
}

// allOf and anyOf combine clauses, returning the one clause there is rather than wrapping a
// single alternative in a bool query that would mean the same thing.
func allOf(queries []esquery.Query) esquery.Query {
	if len(queries) == 1 {
		return queries[0]
	}
	return esquery.NewBoolQuery().Must(queries...)
}

func anyOf(queries []esquery.Query) esquery.Query {
	if len(queries) == 1 {
		return queries[0]
	}
	return esquery.NewBoolQuery().Should(queries...)
}

func refArg(predicate *expression.Call) (reference, error) {
	if len(predicate.Args) != 1 {
		return reference{}, errArity(predicate)
	}
	ref, ok := asReference(predicate.Args[0])
	if !ok {
		return reference{}, fmt.Errorf("%w: %q reads a value on the span",
			tracestore.ErrFilterInvalid, predicate.Op)
	}
	return ref, nil
}

// refAndConstantArgs reads the operands of a comparison. A finalized filter puts the reference
// first (RFC 0005 §5.3), so the second operand is the constant; two references is a valid filter
// this engine has no expression for, and is refused as unsupported rather than misread.
func refAndConstantArgs(predicate *expression.Call) (reference, expression.Expression, error) {
	if len(predicate.Args) != 2 {
		return reference{}, nil, errArity(predicate)
	}
	ref, refOK := asReference(predicate.Args[0])
	if !refOK || isReference(predicate.Args[1]) {
		return reference{}, nil, errRefAgainstConstant(predicate)
	}
	return ref, predicate.Args[1], nil
}

func refAndListArgs(predicate *expression.Call) (reference, *expression.List, error) {
	if len(predicate.Args) != 2 {
		return reference{}, nil, errArity(predicate)
	}
	ref, refOK := asReference(predicate.Args[0])
	list, listOK := predicate.Args[1].(*expression.List)
	if !refOK || !listOK || list == nil {
		return reference{}, nil, errRefAgainstConstant(predicate)
	}
	return ref, list, nil
}

func isReference(term expression.Expression) bool {
	_, ok := asReference(term)
	return ok
}

func errArity(predicate *expression.Call) error {
	return fmt.Errorf("%w: %q cannot take %d arguments",
		tracestore.ErrFilterInvalid, predicate.Op, len(predicate.Args))
}

// errRefAgainstConstant refuses an operand shape the query engine has no expression for.
// Comparing two values read off the same span needs a script, which is too costly to run
// over every candidate span to offer here.
func errRefAgainstConstant(predicate *expression.Call) error {
	return fmt.Errorf("%w: it evaluates %q against a constant only",
		tracestore.ErrFilterUnsupported, predicate.Op)
}

func errUnsupportedLevel(level expression.Level) error {
	return fmt.Errorf("%w: it does not index the %q level", tracestore.ErrFilterUnsupported, level)
}

func errUnsupportedField(ref reference) error {
	return fmt.Errorf("%w: it does not support the built-in field %q of the %q level",
		tracestore.ErrFilterUnsupported, ref.name, ref.level)
}

func errUnorderedValue(op expression.Operator, ref reference) error {
	return fmt.Errorf("%w: it indexes %q as a keyword rather than a number, so it cannot evaluate %q on it",
		tracestore.ErrFilterUnsupported, ref.name, op)
}

// errTypedConstant refuses a constant that declares a type this schema cannot search. A declared
// type is authoritative: the backend must match only the values stored at that type (RFC 0005 §5.4).
// This schema stores every attribute value as text, so it can honor a string declaration but has
// nowhere to search for an integer, a double or a boolean. RFC 0015 adds the typed indexing that
// would make those searchable.
func errTypedConstant(value expression.Expression) error {
	return fmt.Errorf("%w: %s declares a type this schema cannot search",
		tracestore.ErrFilterUnsupported, constantKind(value))
}

// constantKind names a constant in an error, by the wire type it declares rather than by its Go
// type, since the wire type is what a caller wrote.
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
