// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"bytes"
	"encoding/binary"
	"strconv"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var _ Adjuster = (*IPAttributeAdjuster)(nil)

var ipAttributesToCorrect = map[string]struct{}{
	"ip":        {},
	"peer.ipv4": {},
}

// IPAttribute returns an adjuster that replaces numeric "ip" attributes,
// which usually contain IPv4 packed into uint32, with their string
// representation (e.g. "8.8.8.8"").
func IPAttribute() IPAttributeAdjuster {
	return IPAttributeAdjuster{}
}

type IPAttributeAdjuster struct{}

func (ia IPAttributeAdjuster) Adjust(traces ptrace.Traces) {
	resourceSpans := traces.ResourceSpans()
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		ia.adjustAttributes(rs.Resource().Attributes())
		scopeSpans := rs.ScopeSpans()
		for j := 0; j < scopeSpans.Len(); j++ {
			ss := scopeSpans.At(j)
			spans := ss.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				ia.adjustAttributes(span.Attributes())
			}
		}
	}
}

func (IPAttributeAdjuster) adjustAttributes(attributes pcommon.Map) {
	adjusted := make(map[string]string)
	attributes.Range(func(k string, v pcommon.Value) bool {
		if _, ok := ipAttributesToCorrect[k]; !ok {
			return true
		}
		var value uint32
		switch v.Type() {
		case pcommon.ValueTypeInt:
			//nolint: gosec // G115
			value = uint32(v.Int())
		case pcommon.ValueTypeDouble:
			value = uint32(v.Double())
		default:
			return true
		}
		var buf [4]byte
		binary.BigEndian.PutUint32(buf[:], value)
		var sBuf bytes.Buffer
		for i, b := range buf {
			if i > 0 {
				sBuf.WriteRune('.')
			}
			sBuf.WriteString(strconv.FormatUint(uint64(b), 10))
		}
		adjusted[k] = sBuf.String()
		return true
	})
	for k, v := range adjusted {
		attributes.PutStr(k, v)
	}
}
