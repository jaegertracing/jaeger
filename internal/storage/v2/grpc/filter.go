// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	storage "github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// toProtoFilter encodes a query filter for the wire. It is total rather than validating:
// a filter reaches here only after the query service has checked it against what the
// remote backend declared it can evaluate, and an operator or level this build does not
// know is still passed through, because the value sets are open and the backend that
// declared support is the one that has to read them.
func toProtoFilter(filter *tracestore.Call) *storage.Call {
	if filter == nil {
		return nil
	}
	args := make([]*storage.Expression, 0, len(filter.Args))
	for _, arg := range filter.Args {
		args = append(args, toProtoExpression(arg))
	}
	return &storage.Call{
		Op:   string(filter.Op),
		Args: args,
	}
}

func toProtoExpression(expr tracestore.Expression) *storage.Expression {
	switch term := expr.(type) {
	case *tracestore.Reference:
		return &storage.Expression{Term: &storage.Expression_Ref{Ref: &storage.Reference{
			Name:  term.Name,
			Level: string(term.Level),
			Attr:  term.Attr,
		}}}
	case *tracestore.Scalar:
		return &storage.Expression{Term: &storage.Expression_Scalar{Scalar: &storage.Scalar{
			Value: term.Value,
			Type:  string(term.Type),
		}}}
	case *tracestore.List:
		return &storage.Expression{Term: &storage.Expression_List{List: &storage.List{
			Values: term.Values,
			Type:   string(term.Type),
		}}}
	case *tracestore.Call:
		return &storage.Expression{Term: &storage.Expression_Call{Call: toProtoFilter(term)}}
	default:
		return &storage.Expression{}
	}
}

// fromProtoFilter decodes a query filter received from a client. The reader it is handed
// to validates it; a term the client left unset decodes to nil, which the reader refuses
// along with anything else it cannot evaluate.
func fromProtoFilter(filter *storage.Call) *tracestore.Call {
	if filter == nil {
		return nil
	}
	args := make([]tracestore.Expression, 0, len(filter.GetArgs()))
	for _, arg := range filter.GetArgs() {
		if expr := fromProtoExpression(arg); expr != nil {
			args = append(args, expr)
		}
	}
	return &tracestore.Call{
		Op:   tracestore.Operator(filter.GetOp()),
		Args: args,
	}
}

func fromProtoExpression(expr *storage.Expression) tracestore.Expression {
	switch term := expr.GetTerm().(type) {
	case *storage.Expression_Ref:
		return &tracestore.Reference{
			Name:  term.Ref.GetName(),
			Level: tracestore.Level(term.Ref.GetLevel()),
			Attr:  term.Ref.GetAttr(),
		}
	case *storage.Expression_Scalar:
		return &tracestore.Scalar{
			Value: term.Scalar.GetValue(),
			Type:  tracestore.ValueType(term.Scalar.GetType()),
		}
	case *storage.Expression_List:
		return &tracestore.List{
			Values: term.List.GetValues(),
			Type:   tracestore.ValueType(term.List.GetType()),
		}
	case *storage.Expression_Call:
		if term.Call == nil {
			return nil
		}
		return fromProtoFilter(term.Call)
	default:
		return nil
	}
}

func toProtoFilterCapabilities(caps *tracestore.FilterCapabilities) *storage.FilterCapabilities {
	if caps == nil {
		return nil
	}
	levels := make([]string, 0, len(caps.Levels))
	for _, level := range caps.Levels {
		levels = append(levels, string(level))
	}
	operators := make([]string, 0, len(caps.Operators))
	for _, op := range caps.Operators {
		operators = append(operators, string(op))
	}
	return &storage.FilterCapabilities{
		Levels:    levels,
		Operators: operators,
	}
}

func fromProtoFilterCapabilities(caps *storage.FilterCapabilities) *tracestore.FilterCapabilities {
	if caps == nil {
		return nil
	}
	levels := make([]tracestore.Level, 0, len(caps.GetLevels()))
	for _, level := range caps.GetLevels() {
		levels = append(levels, tracestore.Level(level))
	}
	operators := make([]tracestore.Operator, 0, len(caps.GetOperators()))
	for _, op := range caps.GetOperators() {
		operators = append(operators, tracestore.Operator(op))
	}
	return &tracestore.FilterCapabilities{
		Levels:    levels,
		Operators: operators,
	}
}
