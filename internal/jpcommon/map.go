// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jpcommon

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func PcommonMapToPlainMap(attributes pcommon.Map) map[string]string {
	mapAttributes := make(map[string]string)
	attributes.Range(func(k string, v pcommon.Value) bool {
		mapAttributes[k] = v.AsString()
		return true
	})
	return mapAttributes
}

func PlainMapToPcommonMap(attributesMap map[string]string) pcommon.Map {
	attributes := pcommon.NewMap()
	for k, v := range attributesMap {
		attributes.PutStr(k, v)
	}
	return attributes
}
