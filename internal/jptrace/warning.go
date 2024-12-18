// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	// WarningsAttribute is the name of the span attribute where we can
	// store various warnings produced from transformations,
	// such as inbound sanitizers and outbound adjusters.
	// The value type of the attribute is a string slice.
	warningsAttribute = "jaeger.internal.warnings"
)

func AddWarning(span ptrace.Span, warning string) {
	var warnings pcommon.Slice
	if currWarnings, ok := span.Attributes().Get(warningsAttribute); ok {
		warnings = currWarnings.Slice()
	} else {
		warnings = span.Attributes().PutEmptySlice(warningsAttribute)
	}
	warnings.AppendEmpty().SetStr(warning)
}

func GetWarnings(span ptrace.Span) []string {
	if w, ok := span.Attributes().Get(warningsAttribute); ok {
		warnings := []string{}
		ws := w.Slice()
		for i := 0; i < ws.Len(); i++ {
			warnings = append(warnings, ws.At(i).Str())
		}
		return warnings
	}
	return nil
}
