// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jotlp

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	WarningsAttribute = "jaeger.internal.warnings"
)

func AddWarning(span ptrace.Span, warning string) {
	var warnings pcommon.Slice
	if currWarnings, ok := span.Attributes().Get(WarningsAttribute); ok {
		warnings = currWarnings.Slice()
	} else {
		warnings = span.Attributes().PutEmptySlice(WarningsAttribute)
	}
	warnings.AppendEmpty().SetStr(warning)
}
