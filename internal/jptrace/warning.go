// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func AddWarnings(span ptrace.Span, warnings ...string) {
	var w pcommon.Slice
	if currWarnings, ok := span.Attributes().Get(WarningsAttribute); ok {
		if currWarnings.Type() == pcommon.ValueTypeSlice {
			w = currWarnings.Slice()
		} else {
			// The attribute may round-trip through storage backends that do not
			// support slice-typed tags and come back as a plain string
			// (mirrors the malformed-data fallback in GetWarnings).
			prev := currWarnings.AsString()
			w = span.Attributes().PutEmptySlice(WarningsAttribute)
			w.AppendEmpty().SetStr(prev)
		}
	} else {
		w = span.Attributes().PutEmptySlice(WarningsAttribute)
	}
	for _, warning := range warnings {
		w.AppendEmpty().SetStr(warning)
	}
}

func GetWarnings(span ptrace.Span) []string {
	if wa, ok := span.Attributes().Get(WarningsAttribute); ok {
		switch wa.Type() {
		case pcommon.ValueTypeSlice:
			warnings := []string{}
			ws := wa.Slice()
			for i := 0; i < ws.Len(); i++ {
				warnings = append(warnings, ws.At(i).Str())
			}
			return warnings
		default:
			// fallback for malformed data
			return []string{wa.AsString()}
		}
	}
	return nil
}
