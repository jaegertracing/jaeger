// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package expression

import (
	"errors"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
)

// The filter AST crosses two boundaries — the public api_v3 request and the remote-storage
// protocol — and both carry these same messages, so the conversion to and from the storage
// filter lives here once rather than in each of them.

// ToFilter converts a filter received over the wire into the filter AST, and
// reports a structure the AST has no place for. It does not check the filter
// against what any backend can serve, nor whether the operators and levels it names are ones
// this build knows: a caller validates with expression.ValidateFilter, whose errors name the
// filter's own vocabulary rather than the wire's.
func ToFilter(filter *Call) (*expression.Call, error) {
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
	case *Expression_Ref:
		return &expression.Reference{
			Name:  term.Ref.GetName(),
			Level: expression.Level(term.Ref.GetLevel()),
			Attr:  term.Ref.GetAttr(),
		}, nil
	case *Expression_Scalar:
		return &expression.Scalar{
			Value: term.Scalar.GetValue(),
			Type:  expression.ValueType(term.Scalar.GetType()),
		}, nil
	case *Expression_List:
		return &expression.List{
			Values: term.List.GetValues(),
			Type:   expression.ValueType(term.List.GetType()),
		}, nil
	case *Expression_Call:
		nested, err := ToFilter(term.Call)
		if err != nil {
			return nil, err
		}
		if nested == nil {
			return nil, errors.New("filter argument is empty: a call argument must carry a call")
		}
		return nested, nil
	default:
		return nil, errors.New("filter argument is empty: exactly one of ref, scalar, list or call must be set")
	}
}

// FromFilter encodes a filter for the wire. It is total rather than
// validating: a filter reaches here only after the query service has checked it against what
// the receiving backend declared it can evaluate, and an operator or level this build does
// not know is still passed through, because the value sets are open and the backend that
// declared support is the one that has to read them.
func FromFilter(filter *expression.Call) *Call {
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
	case *expression.Reference:
		return &Expression{Term: &Expression_Ref{Ref: &Reference{
			Name:  term.Name,
			Level: string(term.Level),
			Attr:  term.Attr,
		}}}
	case *expression.Scalar:
		return &Expression{Term: &Expression_Scalar{Scalar: &Scalar{
			Value: term.Value,
			Type:  string(term.Type),
		}}}
	case *expression.List:
		return &Expression{Term: &Expression_List{List: &List{
			Values: term.Values,
			Type:   string(term.Type),
		}}}
	case *expression.Call:
		return &Expression{Term: &Expression_Call{Call: FromFilter(term)}}
	default:
		return &Expression{}
	}
}
