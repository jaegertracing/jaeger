// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/hex"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

func convertTraceIDFromDB(dbTraceId dbmodel.TraceID) (pcommon.TraceID, error) {
	var traceId [16]byte
	traceIdHex := string(dbTraceId)
	if len(traceIdHex) > 32 {
		return pcommon.TraceID{}, fmt.Errorf("trace ID from DB is too long: %d chars", len(traceIdHex))
	}
	// Left-pad with zeros to 32 hex chars to handle shorter (e.g. 64-bit) trace IDs.
	if len(traceIdHex) < 32 {
		traceIdHex = strings.Repeat("0", 32-len(traceIdHex)) + traceIdHex
	}
	traceBytes, err := hex.DecodeString(traceIdHex)
	if err != nil {
		return pcommon.TraceID{}, err
	}
	copy(traceId[:], traceBytes)
	return traceId, nil
}

func fromDbSpanId(dbSpanId dbmodel.SpanID) (pcommon.SpanID, error) {
	var spanId [8]byte
	spanIdHex := string(dbSpanId)
	if len(spanIdHex) > 16 {
		return pcommon.SpanID{}, fmt.Errorf("span ID from DB is too long: %d chars", len(spanIdHex))
	}
	// Left-pad with zeros to 16 hex chars to handle shorter span IDs.
	if len(spanIdHex) < 16 {
		spanIdHex = strings.Repeat("0", 16-len(spanIdHex)) + spanIdHex
	}
	spanIdBytes, err := hex.DecodeString(spanIdHex)
	if err != nil {
		return pcommon.SpanID{}, err
	}
	copy(spanId[:], spanIdBytes)
	return spanId, nil
}

// TODO extend DB model to support parent span ID directly
func getParentSpanId(dbSpan *dbmodel.Span) dbmodel.SpanID {
	var followsFromRef *dbmodel.Reference
	for i := range dbSpan.References {
		ref := dbSpan.References[i]
		if ref.TraceID != dbSpan.TraceID {
			continue
		}
		if ref.RefType == dbmodel.ChildOf {
			return ref.SpanID
		}
		if followsFromRef == nil && ref.RefType == dbmodel.FollowsFrom {
			followsFromRef = &ref
		}
	}
	if followsFromRef != nil {
		return followsFromRef.SpanID
	}
	return ""
}
