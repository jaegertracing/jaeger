// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

func convertTraceIDFromDB(dbTraceId dbmodel.TraceID) (pcommon.TraceID, error) {
	return dbTraceId.ToOTEL()
}

func fromDbSpanId(dbSpanId dbmodel.SpanID) (pcommon.SpanID, error) {
	return dbSpanId.ToOTEL()
}

func getParentSpanId(dbSpan *dbmodel.Span) dbmodel.SpanID {
	if dbSpan.ParentSpanID != "" {
		return dbSpan.ParentSpanID
	}
	// Fallback for data written before parentSpanID was populated on the write path.
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
