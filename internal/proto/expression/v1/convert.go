// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// The filter AST crosses two boundaries — the public api_v3 request and the remote-storage
// protocol — and both carry these same generated messages, so the conversion to and from the
// jaeger-idl types lives here once rather than in each of them.

// FromProto converts a filter received over the wire into the filter AST. It fails only where
// the wire says something the AST cannot hold at all — an argument with no term set, a call
// argument carrying no call, or a constant whose spelling is not the type it declares — because
// those decode to nothing rather than to a tree.
//
// It does not validate. Whether the tree is well formed is expression.ValidateFilter's question
// and every wire that decodes a filter has to ask it, but keeping the two apart lets a test
// build a tree the validator rejects, and lets a caller decode a payload it means to inspect
// rather than serve. It says nothing about what a backend can serve either; that is what a
// backend's declared capabilities are for.
func FromProto(filter *Call) (*expression.Call, error) {
	if filter == nil {
		return nil, nil
	}
	args := make([]expression.Expression, 0, len(filter.GetArgs()))
	for _, arg := range filter.GetArgs() {
		expr, err := toFilterExpression(arg)
		if err != nil {
			return nil, err
		}
		args = append(args, expr)
	}
	return &expression.Call{
		Op:   expression.Operator(filter.GetOp()),
		Args: args,
	}, nil
}

func toFilterExpression(expr *Expression) (expression.Expression, error) {
	switch term := expr.GetTerm().(type) {
	case *Expression_Attr:
		return &expression.AttributeRef{
			Key:   term.Attr.GetKey(),
			Level: expression.Level(term.Attr.GetLevel()),
		}, nil
	case *Expression_Field:
		return &expression.FieldRef{
			Name:  term.Field.GetName(),
			Level: expression.Level(term.Field.GetLevel()),
		}, nil
	case *Expression_Nested:
		return &expression.NestedRef{
			Level: expression.Level(term.Nested.GetLevel()),
		}, nil
	case *Expression_Scalar:
		return toFilterConstant(term.Scalar)
	case *Expression_List:
		return &expression.List{
			Values: term.List.GetValues(),
			Type:   expression.ValueType(term.List.GetType()),
		}, nil
	case *Expression_Call:
		nested, err := FromProto(term.Call)
		if err != nil {
			return nil, err
		}
		if nested == nil {
			return nil, errors.New("filter argument is empty: a call argument must carry a call")
		}
		return nested, nil
	default:
		return nil, errors.New("filter argument is empty: exactly one of attr, field, nested, scalar, list or call must be set")
	}
}

// toFilterConstant reads a wire constant as the node that holds it. The type hint is optional and
// authoritative when set (RFC 0005 §5.4), so an unhinted constant becomes the untyped node — a
// duration or an instant among them, since the wire has no hint for either and what a spelling
// like "2s" means is settled by the field it is compared against (expression.ResolveConstants).
func toFilterConstant(scalar *Scalar) (expression.Expression, error) {
	value := scalar.GetValue()
	switch valueType := expression.ValueType(scalar.GetType()); valueType {
	case "":
		return &expression.AnyValue{Value: value}, nil
	case expression.ValueTypeString:
		return &expression.StringValue{Value: value}, nil
	case expression.ValueTypeInt:
		number, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, errNotOfDeclaredType(value, valueType)
		}
		return &expression.IntValue{Value: number}, nil
	case expression.ValueTypeDouble:
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, errNotOfDeclaredType(value, valueType)
		}
		return &expression.DoubleValue{Value: number}, nil
	case expression.ValueTypeBool:
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return nil, errNotOfDeclaredType(value, valueType)
		}
		return &expression.BoolValue{Value: flag}, nil
	default:
		return nil, fmt.Errorf("filter constant %q declares the unknown type %q", value, valueType)
	}
}

func errNotOfDeclaredType(value string, valueType expression.ValueType) error {
	return fmt.Errorf("filter constant %q is not the %q it declares", value, valueType)
}

// ToProto encodes a filter for the wire. It is total rather than validating: a filter reaches
// here only after the query service has checked it against what the receiving backend declared it
// can evaluate, and an operator or level this build does not know is still passed through,
// because the value sets are open and the backend that declared support is the one that has to
// read them.
func ToProto(filter *expression.Call) *Call {
	if filter == nil {
		return nil
	}
	args := make([]*Expression, 0, len(filter.Args))
	for _, arg := range filter.Args {
		args = append(args, fromFilterExpression(arg))
	}
	return &Call{
		Op:   string(filter.Op),
		Args: args,
	}
}

func fromFilterExpression(expr expression.Expression) *Expression {
	switch term := expr.(type) {
	case *expression.AttributeRef:
		return &Expression{Term: &Expression_Attr{Attr: &AttributeReference{
			Key:   term.Key,
			Level: string(term.Level),
		}}}
	case *expression.FieldRef:
		return &Expression{Term: &Expression_Field{Field: &FieldReference{
			Name:  term.Name,
			Level: string(term.Level),
		}}}
	case *expression.NestedRef:
		return &Expression{Term: &Expression_Nested{Nested: &NestedReference{
			Level: string(term.Level),
		}}}
	case *expression.List:
		return &Expression{Term: &Expression_List{List: &List{
			Values: term.Values,
			Type:   string(term.Type),
		}}}
	case *expression.Call:
		return &Expression{Term: &Expression_Call{Call: ToProto(term)}}
	}
	if scalar := fromFilterConstant(expr); scalar != nil {
		return &Expression{Term: &Expression_Scalar{Scalar: scalar}}
	}
	return &Expression{}
}

// fromFilterConstant writes a constant node as the wire's spelling plus the hint that fits it. A
// duration and an instant have no hint of their own, so they travel as an unhinted constant in
// the syntax the field they are compared against is written in — Go duration syntax and RFC 3339
// — which is the spelling the receiving side reads them back from.
func fromFilterConstant(expr expression.Expression) *Scalar {
	switch term := expr.(type) {
	case *expression.AnyValue:
		return &Scalar{Value: term.Value}
	case *expression.StringValue:
		return &Scalar{Value: term.Value, Type: string(expression.ValueTypeString)}
	case *expression.IntValue:
		return &Scalar{Value: strconv.FormatInt(term.Value, 10), Type: string(expression.ValueTypeInt)}
	case *expression.DoubleValue:
		return &Scalar{Value: strconv.FormatFloat(term.Value, 'g', -1, 64), Type: string(expression.ValueTypeDouble)}
	case *expression.BoolValue:
		return &Scalar{Value: strconv.FormatBool(term.Value), Type: string(expression.ValueTypeBool)}
	case *expression.DurationValue:
		return &Scalar{Value: term.Value.String()}
	case *expression.TimestampValue:
		return &Scalar{Value: term.Value.Format(time.RFC3339Nano)}
	default:
		return nil
	}
}
