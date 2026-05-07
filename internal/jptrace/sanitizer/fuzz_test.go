// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func FuzzUTF8Sanitizer(f *testing.F) {
	f.Add("valid string", "another valid")
	f.Add("\xff\xfe\xfd", "invalid utf8")

	f.Fuzz(func(t *testing.T, key string, val string) {
		traces := ptrace.NewTraces()
		rs := traces.ResourceSpans().AppendEmpty()
		ss := rs.ScopeSpans().AppendEmpty()
		span := ss.Spans().AppendEmpty()

		span.SetName(key)
		span.Attributes().PutStr(key, val)

		_ = sanitizeUTF8(traces)
	})
}

func FuzzAttributesSanitizer(f *testing.F) {
	f.Add("key1", "val1")
	f.Add("\xff", "\xfe")

	f.Fuzz(func(t *testing.T, k string, v string) {
		m := pcommon.NewMap()
		m.PutStr(k, v)
		sanitizeAttributes(m)
	})
}
