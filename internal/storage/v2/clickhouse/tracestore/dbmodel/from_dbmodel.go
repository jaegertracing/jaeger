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

// ToPtrace Convert traces from the ClickHouse database model to the  OTel pipeline model.
func ToPtrace(dbTrace Trace) ptrace.Traces {
	trace := ptrace.NewTraces()
	resourceSpans := trace.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()

	span := scopeSpans.Spans().AppendEmpty()
	resourceAttributes := AttributesGroupToMap(span, dbTrace.Resource.Attributes)
	resourceAttributes.CopyTo(resourceSpans.Resource().Attributes())
	scopeAttributes := AttributesGroupToMap(span, dbTrace.Scope.Attributes)
	scopeAttributes.CopyTo(scopeSpans.Scope().Attributes())
	spanAttributes := AttributesGroupToMap(span, dbTrace.Span.Attributes)
	spanAttributes.CopyTo(span.Attributes())

	for i := 0; i < len(dbTrace.Events); i++ {
		event := span.Events().AppendEmpty()
		eventAttributes := AttributesGroupToMap(span, dbTrace.Events[i].Attributes)
		eventAttributes.CopyTo(event.Attributes())
	}

	for i := 0; i < len(dbTrace.Links); i++ {
		link := span.Links().AppendEmpty()
		linkAttributes := AttributesGroupToMap(span, dbTrace.Links[i].Attributes)
		linkAttributes.CopyTo(link.Attributes())
	}
	return trace
}

func AttributesGroupToMap(span ptrace.Span, group AttributesGroup) pcommon.Map {
	result := pcommon.NewMap()
	for i := 0; i < len(group.BoolKeys); i++ {
		key := group.BoolKeys[i]
		value := group.BoolValues[i]
		result.PutBool(key, value)
	}
	for i := 0; i < len(group.IntKeys); i++ {
		key := group.IntKeys[i]
		value := group.IntValues[i]
		result.PutInt(key, value)
	}
	for i := 0; i < len(group.DoubleKeys); i++ {
		key := group.DoubleKeys[i]
		value := group.DoubleValues[i]
		result.PutDouble(key, value)
	}
	for i := 0; i < len(group.StrKeys); i++ {
		key := group.StrKeys[i]
		value := group.StrValues[i]
		result.PutStr(key, value)
	}
	for i := 0; i < len(group.BytesKeys); i++ {
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
