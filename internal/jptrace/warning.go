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
		w = currWarnings.Slice()
	} else {
		w = span.Attributes().PutEmptySlice(WarningsAttribute)
	}
	for _, warning := range warnings {
		w.AppendEmpty().SetStr(warning)
	}
}

func GetWarnings(span ptrace.Span) []string {
	if w, ok := span.Attributes().Get(WarningsAttribute); ok {
		warnings := []string{}
		ws := w.Slice()
		for i := 0; i < ws.Len(); i++ {
			warnings = append(warnings, ws.At(i).Str())
		}
		return warnings
	}
	return nil
}
