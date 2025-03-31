// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"encoding/hex"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

func fromDbTraceId(dbTraceId dbmodel.TraceID) (pcommon.TraceID, error) {
	var traceId [16]byte
	traceBytes, err := hex.DecodeString(string(dbTraceId))
	if err != nil {
		return pcommon.TraceID{}, err
	}
	copy(traceId[:], traceBytes)
	return traceId, nil
}

func fromDbSpanId(dbSpanId dbmodel.SpanID) (pcommon.SpanID, error) {
	var spanId [8]byte
	spanIdBytes, err := hex.DecodeString(string(dbSpanId))
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
