// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"errors"

	"github.com/jaegertracing/jaeger/internal/proto/api_v3"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// toStorageFilter converts the filter of an api_v3 request into the storage filter and
// checks that it is well formed. A request without a filter converts to nil.
func toStorageFilter(filter *api_v3.Call) (*tracestore.Call, error) {
	if filter == nil {
		return nil, nil
	}
	call, err := toStorageCall(filter)
	if err != nil {
		return nil, err
	}
	if err := tracestore.ValidateFilter(call); err != nil {
		return nil, err
	}
	return call, nil
}

func toStorageCall(call *api_v3.Call) (*tracestore.Call, error) {
	args := make([]tracestore.Expression, 0, len(call.GetArgs()))
	for _, arg := range call.GetArgs() {
		expr, err := toStorageExpression(arg)
		if err != nil {
			return nil, err
		}
		args = append(args, expr)
	}
	return &tracestore.Call{
		Op:   tracestore.Operator(call.GetOp()),
		Args: args,
	}, nil
}

func toStorageExpression(expr *api_v3.Expression) (tracestore.Expression, error) {
	switch term := expr.GetTerm().(type) {
	case *api_v3.Expression_Ref:
		return &tracestore.Reference{
			Name:  term.Ref.GetName(),
			Level: tracestore.Level(term.Ref.GetLevel()),
			Attr:  term.Ref.GetAttr(),
		}, nil
	case *api_v3.Expression_Scalar:
		return &tracestore.Scalar{
			Value: term.Scalar.GetValue(),
			Type:  tracestore.ValueType(term.Scalar.GetType()),
		}, nil
	case *api_v3.Expression_List:
		return &tracestore.List{
			Values: term.List.GetValues(),
			Type:   tracestore.ValueType(term.List.GetType()),
		}, nil
	case *api_v3.Expression_Call:
		nested, err := toStorageCall(term.Call)
		if err != nil {
			return nil, err
		}
		return nested, nil
	default:
		return nil, errors.New("filter argument is empty: exactly one of ref, scalar, list or call must be set")
	}
}
