// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jotlp

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	adjusterWarningAttribute = "jaeger.adjuster.warning"
)

func AddWarning(span ptrace.Span, warning string) {
	var warnings pcommon.Slice
	if currWarnings, ok := span.Attributes().Get(adjusterWarningAttribute); ok {
		warnings = currWarnings.Slice()
	} else {
		warnings = span.Attributes().PutEmptySlice(adjusterWarningAttribute)
	}
	warnings.AppendEmpty().SetStr(warning)
}
