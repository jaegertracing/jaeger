// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// ResolveFilterConstants reads every unconstrained constant that is compared against a built-in
// field as that field's declared type, and refuses one whose text will not parse — a
// duration of "banana" is answered at the query boundary rather than passed to a backend to
// interpret. It is stage 3 of RFC 0005 §7 and expects a filter ValidateFilter has accepted.
//
// A constant compared against an *attribute* is left alone, because only storage knows how that
// attribute was written. Resolution rewrites the nodes it changes and returns a new tree rather
// than annotating the one it was given, so nothing it produces can go stale when a query
// interceptor edits a predicate afterwards.
//
// It also puts the reference first in every comparison, so each consumer downstream reads one
// orientation rather than handling both.
func ResolveFilterConstants(filter *expression.Call) (*expression.Call, error) {
	if filter == nil {
		return nil, errors.New("filter is empty")
	}
	return resolveCall(filter, 1)
}

// resolveCall rebuilds a call with its arguments resolved. The arguments it does not rewrite are
// carried over as they are: a term is never modified in place, so sharing one is safe.
//
// It bounds its own recursion rather than trusting that validation ran first, since resolution
// answers for any tree it is given (see ResolveFilterConstants).
func resolveCall(call *expression.Call, depth int) (*expression.Call, error) {
	if call == nil {
		return nil, nil
	}
	if depth > expression.MaxNestingDepth {
		return nil, expression.ErrTooDeeplyNested
	}
	args := make([]expression.Expression, len(call.Args))
	for i, arg := range call.Args {
		nested, ok := arg.(*expression.Call)
		if !ok {
			args[i] = arg
			continue
		}
		resolved, err := resolveCall(nested, depth+1)
		if err != nil {
			return nil, err
		}
		args[i] = resolved
	}
	op := call.Op
	if len(args) == 2 {
		var err error
		switch {
		case isComparison(op):
			if err = resolveComparison(args); err == nil {
				op, args = referenceFirst(op, args)
			}
		case op == expression.OpIn || op == expression.OpNotIn:
			err = checkMembership(args)
		default:
			// Every other two-argument operator carries no constant to resolve.
		}
		if err != nil {
			return nil, err
		}
	}
	return &expression.Call{Op: op, Args: args}, nil
}

// resolveComparison rewrites the unconstrained constant sitting opposite a built-in field. A
// regular expression is not one of the comparisons this runs for, because its pattern stays a
// pattern whatever the field holds, and nor is membership, whose List carries its own elements.
func resolveComparison(args []expression.Expression) error {
	for i, arg := range args {
		ref, ok := arg.(*expression.FieldRef)
		if !ok || ref == nil {
			continue
		}
		other := 1 - i
		field, ok := expression.LookupField(ref.Level, ref.Name)
		if !ok {
			// ValidateFilter refuses a field this API does not define, so there is nothing to
			// resolve against and nothing useful to say about it here.
			continue
		}
		text, ok := textToRead(field.Type, args[other])
		if !ok {
			continue
		}
		value, err := readConstant(field.Type, text)
		if err != nil {
			return fmt.Errorf("cannot compare %s.%s against %q: %w", ref.Level, ref.Name, text, err)
		}
		args[other] = value
	}
	return nil
}

// referenceFirst puts the reference on the left of a comparison. A caller may write the constant
// there instead, and swapping the operands asks the same question as long as an ordered operator
// turns around with them.
func referenceFirst(op expression.Operator, args []expression.Expression) (expression.Operator, []expression.Expression) {
	if !isConstant(args[0]) || isConstant(args[1]) {
		return op, args
	}
	return turnedAround(op), []expression.Expression{args[1], args[0]}
}

func turnedAround(op expression.Operator) expression.Operator {
	switch op {
	case expression.OpGt:
		return expression.OpLt
	case expression.OpLt:
		return expression.OpGt
	case expression.OpGte:
		return expression.OpLte
	case expression.OpLte:
		return expression.OpGte
	default:
		return op
	}
}

// checkMembership reads every element of a list compared against a built-in field as that
// field's type, so a value refused under `gt` is refused under `in` as well. The list is not
// rewritten — it carries its elements as text and there is no typed list node — so this
// only refuses what cannot be read.
//
// A declared element type does not exempt the list. It says how to read the elements, so it has
// to be a type the field could hold, and the elements still have to be readable as it.
func checkMembership(args []expression.Expression) error {
	list, ok := args[1].(*expression.List)
	if !ok || list == nil {
		return nil
	}
	if err := readDeclaredElements(list); err != nil {
		return err
	}
	ref, ok := args[0].(*expression.FieldRef)
	if !ok || ref == nil {
		return nil
	}
	field, ok := expression.LookupField(ref.Level, ref.Name)
	if !ok {
		return nil
	}
	if list.Type != "" && domainOfValueType(list.Type) != domainOfFieldType(field.Type) {
		return fmt.Errorf("cannot compare %s.%s against a list of %s: the field holds %s",
			ref.Level, ref.Name, list.Type, field.Type)
	}
	for _, element := range list.Values {
		if _, err := readConstant(field.Type, element); err != nil {
			return fmt.Errorf("cannot compare %s.%s against %q: %w", ref.Level, ref.Name, element, err)
		}
	}
	return nil
}

// readDeclaredElements reads a list's elements as the type the list declares. That type is what
// the list says its elements are, so it holds wherever the list appears — including beside an
// attribute, where there is no field to check it against.
func readDeclaredElements(list *expression.List) error {
	if list.Type == "" {
		return nil
	}
	for _, element := range list.Values {
		if err := readValue(list.Type, element); err != nil {
			return fmt.Errorf("element %q of a list of %s: %w", element, list.Type, err)
		}
	}
	return nil
}

// ReadFilterElement reads one element of a list as the type the list is read at: Type where the list
// declares one, and otherwise the type of the built-in field it is compared against, which a caller
// passes as fieldType. It is the reading a consumer would otherwise write for itself, and the same
// one finalizing a filter already did, so on a finalized filter it cannot fail.
//
// Beside an attribute there is no field to supply a type, so a caller lowering that comparison
// passes an empty fieldType. An element of a list that declares no type either is under no type
// constraint at all, exactly as an untyped scalar beside an attribute is, and comes back as an
// AnyValue for a backend to match at whatever type the value was stored.
func ReadFilterElement(list *expression.List, fieldType expression.FieldType, element string) (expression.Expression, error) {
	if list == nil {
		return nil, errors.New("list is empty")
	}
	if list.Type == "" {
		if fieldType == "" {
			return &expression.AnyValue{Value: element}, nil
		}
		return readConstant(fieldType, element)
	}
	if err := readValue(list.Type, element); err != nil {
		return nil, fmt.Errorf("element %q of a list of %s: %w", element, list.Type, err)
	}
	return typedValue(list.Type, element)
}

// typedValue builds the node for an element already known to read as its declared type.
func typedValue(t expression.ValueType, element string) (expression.Expression, error) {
	switch t {
	case expression.ValueTypeInt:
		value, err := strconv.ParseInt(element, 10, 64)
		return &expression.IntValue{Value: value}, err
	case expression.ValueTypeDouble:
		value, err := strconv.ParseFloat(element, 64)
		return &expression.DoubleValue{Value: value}, err
	case expression.ValueTypeBool:
		value, err := strconv.ParseBool(element)
		return &expression.BoolValue{Value: value}, err
	default:
		return &expression.StringValue{Value: element}, nil
	}
}

// ReadFilterConstant reads text as the type a built-in field holds, which is what finalizing a filter does
// to every constant compared against one. A consumer that needs the typed value of something the
// wire carried as text — a list element, or a tree that reached it without being finalized — reads
// it here rather than parsing it again itself.
func ReadFilterConstant(t expression.FieldType, text string) (expression.Expression, error) {
	return readConstant(t, text)
}

// readValue reads an element as a declared wire type. A string needs no reading; the others each
// have one form, and anything else is a value the caller cannot have meant.
func readValue(t expression.ValueType, raw string) error {
	var err error
	switch t {
	case expression.ValueTypeInt:
		_, err = strconv.ParseInt(raw, 10, 64)
	case expression.ValueTypeDouble:
		_, err = strconv.ParseFloat(raw, 64)
	case expression.ValueTypeBool:
		_, err = strconv.ParseBool(raw)
	default:
		// A string needs no reading, and an undeclared type constrains nothing.
	}
	return err
}

// textToRead returns the text of a constant that still has to be read as a field's type: an
// untyped constant always, and a string beside a field holding one of a closed set of words, since
// it has to be one of those words. Every other constant already carries its value, and validation
// has refused the ones the field cannot hold.
func textToRead(t expression.FieldType, operand expression.Expression) (string, bool) {
	if isMissing(operand) {
		// Resolution answers for any tree, including one validation would have refused.
		return "", false
	}
	switch value := operand.(type) {
	case *expression.AnyValue:
		return value.Value, true
	case *expression.StringValue:
		if t == expression.FieldTypeSpanKind || t == expression.FieldTypeSpanStatus {
			return value.Value, true
		}
		return "", false
	default:
		// Every other constant already carries its value, so there is no text left to read.
		return "", false
	}
}

// readConstant reads a constant's text as the type a field holds. The two that measure time
// have no wire type of their own, which is why this is the only place they are produced.
func readConstant(t expression.FieldType, raw string) (expression.Expression, error) {
	switch t {
	case expression.FieldTypeString:
		return &expression.StringValue{Value: raw}, nil
	case expression.FieldTypeDuration:
		value, err := time.ParseDuration(raw)
		if err != nil {
			return nil, err
		}
		return &expression.DurationValue{Value: value}, nil
	case expression.FieldTypeTimestamp:
		value, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, err
		}
		return &expression.TimestampValue{Value: value}, nil
	case expression.FieldTypeSpanKind, expression.FieldTypeSpanStatus:
		return readWord(raw, wordsOf(t))
	default:
		return nil, fmt.Errorf("no rule for reading a constant as %q", t)
	}
}

// wordsOf names the closed set a word-valued field holds.
func wordsOf(t expression.FieldType) []string {
	if t == expression.FieldTypeSpanKind {
		return expression.SpanKinds()
	}
	return expression.SpanStatuses()
}

// readWord reads a constant that has to be one of a closed set of words. The set is small
// enough to name in the error, which is the whole value of refusing here rather than letting a
// backend match nothing.
func readWord(raw string, words []string) (expression.Expression, error) {
	if slices.Contains(words, raw) {
		return &expression.StringValue{Value: raw}, nil
	}
	return nil, fmt.Errorf("not one of %s", strings.Join(words, ", "))
}

// isComparison reports whether an operator compares its two operands by value, which is what
// makes a built-in field's type the type of the constant beside it.
func isComparison(op expression.Operator) bool {
	switch op {
	case expression.OpEq, expression.OpNe, expression.OpGt, expression.OpLt, expression.OpGte, expression.OpLte:
		return true
	default:
		return false
	}
}
