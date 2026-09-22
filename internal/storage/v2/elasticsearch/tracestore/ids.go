// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

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
