// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"fmt"
	"reflect"
	"regexp/syntax"
	"slices"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// ValidateFilter checks that a filter is well formed: every operator is one this package
// defines and has the number and kind of arguments it takes, every level and value type is a
// defined one, every reference names something, a reference to a built-in field names one this
// API defines (see Field), and the quantifier's binding rules hold (RFC 0005 §5.5).
//
// What it deliberately leaves alone is a constant's text — whether "banana" is a duration
// is answered by ResolveFilterConstants, which knows the field it is compared against — and which of
// the valid things a given backend can serve, which is what a backend's declared capabilities
// are for.
func ValidateFilter(filter *expression.Call) error {
	if filter == nil {
		return errors.New("filter is empty")
	}
	return validateCall(filter, nil, 1)
}

// validateCall checks one call. quantified carries the collection levels of the enclosing
// OpSome calls, which is what lets a nested quantifier over an already-bound level be refused, and
// depth is how many calls deep this one sits, counting itself.
func validateCall(call *expression.Call, quantified []expression.Level, depth int) error {
	if call == nil {
		return errors.New("filter has a missing predicate")
	}
	if depth > expression.MaxNestingDepth {
		return expression.ErrTooDeeplyNested
	}
	switch call.Op {
	case expression.OpAnd, expression.OpOr:
		if len(call.Args) < 2 {
			return fmt.Errorf("operator %q takes at least two arguments, got %d", call.Op, len(call.Args))
		}
		return validatePredicateArgs(call, quantified, depth)
	case expression.OpNot:
		if err := wantArgs(call, 1); err != nil {
			return err
		}
		return validatePredicateArgs(call, quantified, depth)
	case expression.OpExists:
		if err := wantArgs(call, 1); err != nil {
			return err
		}
		return validateReference(call.Op, call.Args[0])
	case expression.OpSome:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		return validateSome(call, quantified, depth)
	case expression.OpIn, expression.OpNotIn:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		if err := validateSubject(call.Op, call.Args[0], quantified); err != nil {
			return err
		}
		list, ok := call.Args[1].(*expression.List)
		if !ok || list == nil {
			return fmt.Errorf("operator %q takes a list as its second argument, got %s", call.Op, termName(call.Args[1]))
		}
		if len(list.Values) == 0 {
			// Membership in nothing matches nothing, so the query asks for an empty result in a
			// way that reads like an oversight. Refusing says so.
			return fmt.Errorf("operator %q takes a list with at least one element", call.Op)
		}
		return validateValueType(list.Type)
	case expression.OpRegex:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		if err := validateSubject(call.Op, call.Args[0], quantified); err != nil {
			return err
		}
		if err := validateRegexSubject(call.Args[0]); err != nil {
			return err
		}
		pattern, ok := patternText(call.Args[1])
		if !ok {
			return fmt.Errorf("operator %q takes a constant string as its pattern, got %s", call.Op, termName(call.Args[1]))
		}
		return validatePattern(pattern)
	case expression.OpEq, expression.OpNe:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		return validateComparison(call, quantified)
	case expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte:
		if err := wantArgs(call, 2); err != nil {
			return err
		}
		return validateOrderedComparison(call, quantified)
	default:
		return fmt.Errorf("unknown filter operator %q", call.Op)
	}
}

// wantArgs checks an operator's arity in the case that knows it, beside the check on what kind
// of arguments it takes — which is the part worth reading.
func wantArgs(call *expression.Call, n int) error {
	if len(call.Args) != n {
		return fmt.Errorf("operator %q takes %d argument(s), got %d", call.Op, n, len(call.Args))
	}
	return nil
}

// validatePredicateArgs checks the arguments of a boolean combinator, each of which
// must itself be a predicate rather than a bare reference or constant.
func validatePredicateArgs(call *expression.Call, quantified []expression.Level, depth int) error {
	for _, arg := range call.Args {
		nested, ok := arg.(*expression.Call)
		if !ok {
			return fmt.Errorf("operator %q takes predicates as arguments, got %s", call.Op, termName(arg))
		}
		if err := validateCall(nested, quantified, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// validateSome checks the existential quantifier: it binds one element of a span's
// events or links, so its first argument names that collection and its second is the
// predicate evaluated against the bound element.
func validateSome(call *expression.Call, quantified []expression.Level, depth int) error {
	ref, ok := call.Args[0].(*expression.NestedRef)
	if !ok || ref == nil {
		return fmt.Errorf("operator %q takes a collection reference as its first argument, got %s", call.Op, termName(call.Args[0]))
	}
	if ref.Level != expression.LevelEvent && ref.Level != expression.LevelLink {
		return fmt.Errorf("operator %q quantifies over %q or %q, got level %q", call.Op, expression.LevelEvent, expression.LevelLink, ref.Level)
	}
	// RFC 0005 §5.5 rule 4: whether an inner quantifier shadows the outer one, and whether its
	// predicate may reach back to the outer element, are questions this version does not answer,
	// so it refuses the query rather than answering one of them by accident.
	if slices.Contains(quantified, ref.Level) {
		return fmt.Errorf("operator %q is already quantifying over %q, and this version does not define what a nested one would bind", call.Op, ref.Level)
	}
	predicate, ok := call.Args[1].(*expression.Call)
	if !ok {
		return fmt.Errorf("operator %q takes a predicate as its second argument, got %s", call.Op, termName(call.Args[1]))
	}
	return validateCall(predicate, append(slices.Clone(quantified), ref.Level), depth+1)
}

// validateComparison checks the two operands of a comparison. Each names a value on the span or
// supplies a constant, and the two have to hold the same kind of value: a duration against a name
// is a comparison no backend can answer, so it is refused here rather than lowered. Whether either
// operand is a reference does not come into it — `span.startTime < span.endTime` compares two
// instants, and two attributes hold whatever storage wrote, which is compatible with anything.
func validateComparison(call *expression.Call, quantified []expression.Level) error {
	for _, arg := range call.Args {
		if err := validateOperand(call.Op, arg, quantified); err != nil {
			return err
		}
	}
	if err := validateTimeConstant(call.Op, call.Args); err != nil {
		return err
	}
	left, right := domainOfOperand(call.Args[0]), domainOfOperand(call.Args[1])
	if left != domainUnknown && right != domainUnknown && left != right {
		return fmt.Errorf("operator %q compares %s against %s, which hold different kinds of value",
			call.Op, describe(call.Args[0]), describe(call.Args[1]))
	}
	return nil
}

// validateTimeConstant refuses a duration or an instant compared against an attribute. The wire
// carries neither type (§5.4), so one survives only where a built-in field's declared type
// rebuilds it.
// An attribute declares nothing, so the constant would come back untyped and ask the backend a
// different question; comparing the attribute against the plain string asks that one directly.
func validateTimeConstant(op expression.Operator, args []expression.Expression) error {
	for i, arg := range args {
		if _, ok := arg.(*expression.AttributeRef); !ok {
			continue
		}
		switch args[1-i].(type) {
		case *expression.DurationValue:
			return errNoWireSpelling(op, args[1-i], "duration")
		case *expression.TimestampValue:
			return errNoWireSpelling(op, args[1-i], "timestamp")
		}
	}
	return nil
}

func errNoWireSpelling(op expression.Operator, constant expression.Expression, kind string) error {
	return fmt.Errorf("operator %q compares %s against an attribute, and the wire has no %s type",
		op, termName(constant), kind)
}

// validateOrderedComparison adds the one question ordering asks beyond a comparison: whether the
// values have an order to be compared within. Text does, lexicographically, which is a real query
// — `span.name > "m"` asks for the names that sort after it.
func validateOrderedComparison(call *expression.Call, quantified []expression.Level) error {
	if err := validateComparison(call, quantified); err != nil {
		return err
	}
	for _, arg := range call.Args {
		if !orderable(arg) {
			return fmt.Errorf("operator %q has no ordering for %s", call.Op, describe(arg))
		}
	}
	return nil
}

// validateOperand checks a value an operator compares. A call is not one: no operator in this
// vocabulary has a result type, so there is nothing to say about what comparing one would mean.
// An operator that takes a call result — a future extraction function, say — arrives with its
// signature declared rather than through this door (§5.3).
func validateOperand(op expression.Operator, arg expression.Expression, _ []expression.Level) error {
	switch term := arg.(type) {
	case *expression.AttributeRef:
		return validateAttributeRef(term)
	case *expression.FieldRef:
		return validateFieldRef(term)
	case *expression.NestedRef:
		return errCollectionOutOfPlace()
	}
	if isConstant(arg) {
		return nil
	}
	return fmt.Errorf("operator %q compares a reference or a constant, got %s", op, termName(arg))
}

// validateSubject checks the operand an operator reads a value from rather than supplies one
// to: the left-hand side of membership and of a regular expression.
func validateSubject(op expression.Operator, arg expression.Expression, _ []expression.Level) error {
	return validateReference(op, arg)
}

// validateReference checks an argument that has to name a value on the span.
func validateReference(op expression.Operator, arg expression.Expression) error {
	switch term := arg.(type) {
	case *expression.AttributeRef:
		return validateAttributeRef(term)
	case *expression.FieldRef:
		return validateFieldRef(term)
	case *expression.NestedRef:
		return errCollectionOutOfPlace()
	default:
		return fmt.Errorf("operator %q takes a reference, got %s", op, termName(arg))
	}
}

func validateAttributeRef(ref *expression.AttributeRef) error {
	if ref == nil {
		return errors.New("filter has a missing reference")
	}
	if ref.Level != "" && !ref.Level.Valid() {
		return fmt.Errorf("unknown filter level %q", ref.Level)
	}
	if ref.Key == "" {
		return errors.New("attribute reference has no key")
	}
	return nil
}

func validateFieldRef(ref *expression.FieldRef) error {
	if ref == nil {
		return errors.New("filter has a missing reference")
	}
	// An empty level is the unqualified attribute search, and no built-in field has an
	// unqualified form, so there is nothing for a field reference to mean without one.
	if ref.Level == "" {
		return errors.New("field reference has no level, and a built-in field belongs to one")
	}
	if !ref.Level.Valid() {
		return fmt.Errorf("unknown filter level %q", ref.Level)
	}
	if ref.Name == "" {
		return errors.New("field reference has no name")
	}
	if _, ok := expression.LookupField(ref.Level, ref.Name); !ok {
		return fmt.Errorf("unknown built-in field %q at the %q level; name an attribute to match a tag of that name instead",
			ref.Name, ref.Level)
	}
	return nil
}

// errCollectionOutOfPlace refuses a collection reference anywhere but the one place it means
// something. A collection is many values rather than one, so nothing else can read it.
func errCollectionOutOfPlace() error {
	return fmt.Errorf("a collection reference is only the first argument of %q", expression.OpSome)
}

func validateValueType(t expression.ValueType) error {
	if t != "" && !t.Valid() {
		return fmt.Errorf("unknown filter value type %q", t)
	}
	return nil
}

// isConstant reports whether a term is a single constant value. A List is not one: it is only
// ever the right-hand side of membership, never a value an operator reads or compares. Neither is
// a nil pointer of a constant type: it names a kind of value while holding none, so accepting it
// would hand every stage below something to dereference.
func isConstant(e expression.Expression) bool {
	if isMissing(e) {
		return false
	}
	switch e.(type) {
	case *expression.AnyValue, *expression.StringValue, *expression.IntValue, *expression.DoubleValue, *expression.BoolValue, *expression.DurationValue, *expression.TimestampValue:
		return true
	default:
		return false
	}
}

// validateRegexSubject refuses a subject a pattern has nothing to match against. A string field,
// a word-valued field and an attribute all hold text; a duration or a timestamp does not, and
// nothing in this API says what text a pattern would be matched against.
func validateRegexSubject(subject expression.Expression) error {
	ref, ok := subject.(*expression.FieldRef)
	if !ok || ref == nil {
		return nil
	}
	field, _ := expression.LookupField(ref.Level, ref.Name)
	switch field.Type {
	case expression.FieldTypeDuration, expression.FieldTypeTimestamp:
		return fmt.Errorf("operator %q matches text, and %s.%s holds a %s",
			expression.OpRegex, ref.Level, ref.Name, field.Type)
	}
	return nil
}

// domain is the kind of value a term holds, which is what decides whether two operands can be
// compared at all. Nothing has said what an untyped constant or an attribute holds, so those are
// unknown and compare against anything.
//
// Whether a domain has an order is a separate question, and the answer can differ within one: the
// two word-valued fields hold text but have no useful order (see orderable).
type domain int

const (
	domainUnknown domain = iota
	domainNumber
	domainDuration
	domainTimestamp
	domainText
	domainBool
)

// domainOf reads the kind of value a constant holds.
func domainOf(e expression.Expression) domain {
	switch e.(type) {
	case *expression.IntValue, *expression.DoubleValue:
		return domainNumber
	case *expression.DurationValue:
		return domainDuration
	case *expression.TimestampValue:
		return domainTimestamp
	case *expression.StringValue:
		return domainText
	case *expression.BoolValue:
		return domainBool
	default:
		return domainUnknown
	}
}

// domainOfOperand reads the kind of value either side of a comparison holds. A built-in field
// holds what its declared type says; an attribute holds whatever storage wrote there, which is
// not this API's to know.
func domainOfOperand(e expression.Expression) domain {
	if ref, ok := e.(*expression.FieldRef); ok && ref != nil {
		// A field this API does not define is refused before an ordering is asked about, so the
		// zero Field's empty type is only ever reached by a caller checking one term directly.
		field, _ := expression.LookupField(ref.Level, ref.Name)
		return domainOfFieldType(field.Type)
	}
	if _, ok := e.(*expression.AttributeRef); ok {
		return domainUnknown
	}
	return domainOf(e)
}

// domainOfValueType reads the kind of value a declared wire type names, which is what a list's
// element type says about its elements.
func domainOfValueType(t expression.ValueType) domain {
	switch t {
	case expression.ValueTypeInt, expression.ValueTypeDouble:
		return domainNumber
	case expression.ValueTypeString:
		return domainText
	case expression.ValueTypeBool:
		return domainBool
	default:
		return domainUnknown
	}
}

// domainOfFieldType reads the kind of value a built-in field holds. A field holding one of a
// closed set of words holds text, which is what makes a list of strings the right list for it.
func domainOfFieldType(t expression.FieldType) domain {
	switch t {
	case expression.FieldTypeDuration:
		return domainDuration
	case expression.FieldTypeTimestamp:
		return domainTimestamp
	case expression.FieldTypeString, expression.FieldTypeSpanKind, expression.FieldTypeSpanStatus:
		return domainText
	default:
		return domainUnknown
	}
}

// orderable reports whether an operand has an order to be compared within. Two do not: a boolean,
// and a field holding one of a closed set of words, because the kinds that sort after "server" is
// not a question about span kinds.
func orderable(e expression.Expression) bool {
	if ref, ok := e.(*expression.FieldRef); ok && ref != nil {
		field, _ := expression.LookupField(ref.Level, ref.Name)
		return field.Type != expression.FieldTypeSpanKind && field.Type != expression.FieldTypeSpanStatus
	}
	return domainOf(e) != domainBool
}

// validatePattern checks a regular expression. RFC 0005 §5.3 makes it RE2 syntax, matched anywhere
// in the value and case-sensitively, so a pattern that will not parse is refused here rather than
// by whichever backend received it.
func validatePattern(pattern string) error {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return fmt.Errorf("operator %q takes a pattern in RE2 syntax: %w", expression.OpRegex, err)
	}
	return checkPortable(parsed)
}

// checkPortable refuses the constructs the backends this lowers to do not all have. Elasticsearch,
// for one, reads `^` as a literal caret rather than as an anchor, so a pattern using it would be
// answered differently by each backend instead of being refused by the ones that cannot honor it.
func checkPortable(re *syntax.Regexp) error {
	switch re.Op {
	case syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText, syntax.OpEndText:
		return fmt.Errorf("operator %q matches anywhere in the value, so a pattern cannot anchor itself", expression.OpRegex)
	case syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return fmt.Errorf("operator %q takes a pattern without word boundaries", expression.OpRegex)
	}
	if re.Flags&syntax.NonGreedy != 0 {
		return fmt.Errorf("operator %q asks whether the value matches, so a quantifier cannot be lazy", expression.OpRegex)
	}
	if re.Flags&syntax.FoldCase != 0 {
		return fmt.Errorf("operator %q matches case-sensitively, so a pattern cannot fold case", expression.OpRegex)
	}
	for _, sub := range re.Sub {
		if err := checkPortable(sub); err != nil {
			return err
		}
	}
	return nil
}

// patternText returns the text of a constant that can serve as a regular expression. An untyped
// constant can: a pattern is written as a bare string and carries no wire hint.
func patternText(e expression.Expression) (string, bool) {
	if isMissing(e) {
		return "", false
	}
	switch value := e.(type) {
	case *expression.AnyValue:
		return value.Value, true
	case *expression.StringValue:
		return value.Value, true
	}
	return "", false
}

// isMissing reports whether a term holds nothing: either no term at all, or a nil pointer of one
// of the term types, which reads as a term of that type through the Expression interface. It is
// reflection rather than a ten-case type switch because every term type has the same answer, and a
// term added later has to inherit it rather than be remembered here.
func isMissing(e expression.Expression) bool {
	if e == nil {
		return true
	}
	value := reflect.ValueOf(e)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

// describe names an operand the way a caller wrote it, so an error about two operands points at
// which ones. A built-in field is worth naming exactly; anything else is named by its kind.
func describe(e expression.Expression) string {
	if ref, ok := e.(*expression.FieldRef); ok && ref != nil {
		return fmt.Sprintf("%s.%s", ref.Level, ref.Name)
	}
	return termName(e)
}

// termName names the kind of a term for an error message.
func termName(e expression.Expression) string {
	if isMissing(e) {
		// A nil pointer of a term type would otherwise be named after the type it points at, so an
		// error would read "got a list" about an argument that carries no list.
		return "an empty term"
	}
	switch e.(type) {
	case *expression.AttributeRef:
		return "an attribute reference"
	case *expression.FieldRef:
		return "a field reference"
	case *expression.NestedRef:
		return "a collection reference"
	case *expression.AnyValue:
		return "an untyped constant"
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
	case *expression.List:
		return "a list"
	case *expression.Call:
		return "a predicate"
	default:
		return "an unknown term"
	}
}
