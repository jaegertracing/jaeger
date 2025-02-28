// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0
package jptrace

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
)

const (
	// WarningsAttribute is the name of the span attribute where we can
	// store various warnings produced from transformations,
	// such as inbound sanitizers and outbound adjusters.
	// The value type of the attribute is a string slice.
	WarningsAttribute = "@jaeger@warnings"
	// FormatAttribute is a key for span attribute that records the original
	// wire format in which the span was received by Jaeger,
	// e.g. proto, thrift, json.
	FormatAttribute = "@jaeger@format"
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
