// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	storage "github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

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
	levels := make([]expression.Level, 0, len(caps.GetLevels()))
	for _, level := range caps.GetLevels() {
		levels = append(levels, expression.Level(level))
	}
	operators := make([]expression.Operator, 0, len(caps.GetOperators()))
	for _, op := range caps.GetOperators() {
		operators = append(operators, expression.Operator(op))
	}
	return &tracestore.FilterCapabilities{
		Levels:    levels,
		Operators: operators,
	}
}
