// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func StringToValueType(vt string) pcommon.ValueType {
	switch strings.ToLower(vt) {
	case "bool":
		return pcommon.ValueTypeBool
	case "double":
		return pcommon.ValueTypeDouble
	case "int":
		return pcommon.ValueTypeInt
	case "str":
		return pcommon.ValueTypeStr
	case "bytes":
		return pcommon.ValueTypeBytes
	case "map":
		return pcommon.ValueTypeMap
	case "slice":
		return pcommon.ValueTypeSlice
	default:
		return pcommon.ValueTypeEmpty
	}
}

func ValueTypeToString(vt pcommon.ValueType) string {
	if vt == pcommon.ValueTypeEmpty {
		return ""
	}
	return strings.ToLower(vt.String())
}
