// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/base64"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

// FromDBModel Convert traces from the ClickHouse database model to the  OTel pipeline model.
func FromDBModel(dbTrace Trace) ptrace.Traces {
	trace := ptrace.NewTraces()
	resourceSpans := trace.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()

	span := scopeSpans.Spans().AppendEmpty()
	resourceAttributes := attributesGroupToMap(span, dbTrace.Resource.Attributes)
	resourceAttributes.CopyTo(resourceSpans.Resource().Attributes())
	scopeAttributes := attributesGroupToMap(span, dbTrace.Scope.Attributes)
	scopeAttributes.CopyTo(scopeSpans.Scope().Attributes())
	spanAttributes := attributesGroupToMap(span, dbTrace.Span.Attributes)
	spanAttributes.CopyTo(span.Attributes())

	for i := range dbTrace.Events {
		event := span.Events().AppendEmpty()
		eventAttributes := attributesGroupToMap(span, dbTrace.Events[i].Attributes)
		eventAttributes.CopyTo(event.Attributes())
	}

	for i := range dbTrace.Links {
		link := span.Links().AppendEmpty()
		linkAttributes := attributesGroupToMap(span, dbTrace.Links[i].Attributes)
		linkAttributes.CopyTo(link.Attributes())
	}
	return trace
}

func attributesGroupToMap(span ptrace.Span, group AttributesGroup) pcommon.Map {
	result := pcommon.NewMap()
	for i := range group.BoolKeys {
		key := group.BoolKeys[i]
		value := group.BoolValues[i]
		result.PutBool(key, value)
	}
	for i := range group.IntKeys {
		key := group.IntKeys[i]
		value := group.IntValues[i]
		result.PutInt(key, value)
	}
	for i := range group.DoubleKeys {
		key := group.DoubleKeys[i]
		value := group.DoubleValues[i]
		result.PutDouble(key, value)
	}
	for i := range group.StrKeys {
		key := group.StrKeys[i]
		value := group.StrValues[i]
		result.PutStr(key, value)
	}
	for i := range group.BytesKeys {
		key := group.BytesKeys[i]
		value := group.BytesValues[i]
		bts, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			jptrace.AddWarnings(span, fmt.Sprintf("failed to decode bytes value: %v", err))
		}
		result.PutEmptyBytes(key).FromRaw(bts)
	}
	return result
}
