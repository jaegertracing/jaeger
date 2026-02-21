// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/jaegerproto_to_traces.go

package tracestore

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	idutils "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/core/xidutils"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

var errType = errors.New("invalid type")

// FromDBModel converts dbmodel.Span to ptrace.Traces
func FromDBModel(spans []dbmodel.Span) ptrace.Traces {
	traceData := ptrace.NewTraces()
	if len(spans) == 0 {
		return traceData
	}
	resourceSpans := traceData.ResourceSpans()
	resourceSpans.EnsureCapacity(len(spans))
	dbSpansToSpans(spans, resourceSpans)
	return traceData
}

func dbSpansToSpans(dbSpans []dbmodel.Span, resourceSpans ptrace.ResourceSpansSlice) {
	for i := range dbSpans {
		span := &dbSpans[i]
		resourceSpan := resourceSpans.AppendEmpty()
		dbProcessToResource(span.Process, resourceSpan.Resource())
		scopeSpans := resourceSpan.ScopeSpans()
		scopeSpan := scopeSpans.AppendEmpty()
		dbSpanToScope(span, scopeSpan)
		dbSpanToSpan(span, scopeSpan.Spans().AppendEmpty())
	}
}

func dbProcessToResource(process dbmodel.Process, resource pcommon.Resource) {
	serviceName := process.ServiceName
	tags := process.Tags
	if serviceName == "" && tags == nil {
		return
	}
	attrs := resource.Attributes()
	if serviceName != "" && serviceName != noServiceName {
		attrs.EnsureCapacity(len(tags) + 1)
		attrs.PutStr(otelsemconv.ServiceNameKey, serviceName)
	} else {
		attrs.EnsureCapacity(len(tags))
	}
	dbTagsToAttributes(tags, attrs)
}

func dbSpanToSpan(dbspan *dbmodel.Span, span ptrace.Span) {
	span.SetTraceID(pcommon.TraceID(dbspan.TraceID))
	//nolint:gosec // G115 // we only care about bits, not the interpretation as integer, and this conversion is bitwise lossless
	span.SetSpanID(idutils.UInt64ToSpanID(uint64(dbspan.SpanID)))
	span.SetName(dbspan.OperationName)
	//nolint:gosec // G115 // dbspan.Flags is guaranteed non-negative by schema constraints
	span.SetFlags(uint32(dbspan.Flags))
	//nolint:gosec // G115 // epoch microseconds are semantically non-negative, safe conversion to uint64
	span.SetStartTimestamp(dbTimeStampToOTLPTimeStamp(uint64(dbspan.StartTime)))
	//nolint:gosec // G115 // dbspan.StartTime and dbspan.Duration is guaranteed non-negative by schema constraints
	span.SetEndTimestamp(dbTimeStampToOTLPTimeStamp(uint64(dbspan.StartTime + dbspan.Duration)))

	parentSpanID := dbspan.ParentID
	if parentSpanID != 0 {
		//nolint:gosec // G115 // bit-preserving uint64<->int64 conversion for opaque span ID
		span.SetParentSpanID(idutils.UInt64ToSpanID(uint64(parentSpanID)))
	}

	attrs := span.Attributes()
	attrs.EnsureCapacity(len(dbspan.Tags))
	dbTagsToAttributes(dbspan.Tags, attrs)
	if spanKindAttr, ok := attrs.Get(model.SpanKindKey); ok {
		span.SetKind(jSpanKindToInternal(spanKindAttr.Str()))
		attrs.Remove(model.SpanKindKey)
	}
	setSpanStatus(attrs, span)

	span.TraceState().FromRaw(getTraceStateFromAttrs(attrs))

	// drop the attributes slice if all of them were replaced during translation
	if attrs.Len() == 0 {
		attrs.Clear()
	}

	dbLogsToSpanEvents(dbspan.Logs, span.Events())
	dbReferencesToSpanLinks(dbspan.Refs, parentSpanID, span.Links())
}

func dbTagsToAttributes(tags []dbmodel.KeyValue, attributes pcommon.Map) {
	for _, tag := range tags {
		switch tag.ValueType {
		case dbmodel.StringType:
			attributes.PutStr(tag.Key, tag.ValueString)
		case dbmodel.BoolType:
			attributes.PutBool(tag.Key, tag.ValueBool)
		case dbmodel.Int64Type:
			attributes.PutInt(tag.Key, tag.ValueInt64)
		case dbmodel.Float64Type:
			attributes.PutDouble(tag.Key, tag.ValueFloat64)
		case dbmodel.BinaryType:
			attributes.PutEmptyBytes(tag.Key).FromRaw(tag.ValueBinary)
		default:
			attributes.PutStr(tag.Key, fmt.Sprintf("<Unknown Jaeger TagType %q>", tag.ValueType))
		}
	}
}

func setSpanStatus(attrs pcommon.Map, span ptrace.Span) {
	dest := span.Status()
	statusCode := ptrace.StatusCodeUnset
	statusMessage := ""
	statusExists := false

	if errorVal, ok := attrs.Get(tagError); ok && errorVal.Type() == pcommon.ValueTypeBool {
		if errorVal.Bool() {
			statusCode = ptrace.StatusCodeError
			attrs.Remove(tagError)
			statusExists = true

			if desc, ok := extractStatusDescFromAttr(attrs); ok {
				statusMessage = desc
			} else if descAttr, ok := attrs.Get(tagHTTPStatusMsg); ok {
				statusMessage = descAttr.Str()
			}
		}
	}

	if codeAttr, ok := attrs.Get(otelsemconv.OtelStatusCode); ok {
		if !statusExists {
			// The error tag is the ultimate truth for a Jaeger spans' error
			// status. Only parse the otel.status_code tag if the error tag is
			// not set to true.
			statusExists = true
			if strings.ToUpper(codeAttr.Str()) == statusOk {
				statusCode = ptrace.StatusCodeOk
			} else if strings.ToUpper(codeAttr.Str()) == statusError {
				statusCode = ptrace.StatusCodeError
			}
			if desc, ok := extractStatusDescFromAttr(attrs); ok {
				statusMessage = desc
			}
		}
		// Regardless of error tag value, remove the otel.status_code tag. The
		// otel.status_message tag will have already been removed if
		// statusExists is true.
		attrs.Remove(otelsemconv.OtelStatusCode)
	} else if httpCodeAttr, ok := attrs.Get(otelsemconv.HTTPResponseStatusCodeKey); !statusExists && ok {
		// Fallback to introspecting if this span represents a failed HTTP
		// request or response, but again, only do so if the `error` tag was
		// not set to true and no explicit status was sent.
		if code, err := getStatusCodeFromHTTPStatusAttr(httpCodeAttr, span.Kind()); err == nil {
			if code != ptrace.StatusCodeUnset {
				statusExists = true
				statusCode = code
			}

			if msgAttr, ok := attrs.Get(tagHTTPStatusMsg); ok {
				statusMessage = msgAttr.Str()
			}
		}
	}

	if statusExists {
		dest.SetCode(statusCode)
		dest.SetMessage(statusMessage)
	}
}

// extractStatusDescFromAttr returns the OTel status description from attrs
// along with true if it is set. Otherwise, an empty string and false are
// returned. The OTel status description attribute is deleted from attrs in
// the process.
func extractStatusDescFromAttr(attrs pcommon.Map) (string, bool) {
	if msgAttr, ok := attrs.Get(otelsemconv.OtelStatusDescription); ok {
		msg := msgAttr.Str()
		attrs.Remove(otelsemconv.OtelStatusDescription)
		return msg, true
	}
	return "", false
}

// codeFromAttr returns the integer code value from attrVal. An error is
// returned if the code is not represented by an integer or string value in
// the attrVal or the value is outside the bounds of an int representation.
func codeFromAttr(attrVal pcommon.Value) (int64, error) {
	var val int64
	switch attrVal.Type() {
	case pcommon.ValueTypeInt:
		val = attrVal.Int()
	case pcommon.ValueTypeStr:
		var err error
		val, err = strconv.ParseInt(attrVal.Str(), 10, 0)
		if err != nil {
			return 0, err
		}
	default:
		return 0, fmt.Errorf("%w: %s", errType, attrVal.Type().String())
	}
	return val, nil
}

func getStatusCodeFromHTTPStatusAttr(attrVal pcommon.Value, kind ptrace.SpanKind) (ptrace.StatusCode, error) {
	statusCode, err := codeFromAttr(attrVal)
	if err != nil {
		return ptrace.StatusCodeUnset, err
	}

	// For HTTP status codes in the 4xx range span status MUST be left unset
	// in case of SpanKind.SERVER and MUST be set to Error in case of SpanKind.CLIENT.
	// For HTTP status codes in the 5xx range, as well as any other code the client
	// failed to interpret, span status MUST be set to Error.
	if statusCode >= 400 && statusCode < 500 {
		switch kind {
		case ptrace.SpanKindClient:
			return ptrace.StatusCodeError, nil
		case ptrace.SpanKindServer:
			return ptrace.StatusCodeUnset, nil
		}
	}

	return statusCodeFromHTTP(statusCode), nil
}

// StatusCodeFromHTTP takes an HTTP status code and return the appropriate OpenTelemetry status code
// See: https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/semantic_conventions/http.md#status
func statusCodeFromHTTP(httpStatusCode int64) ptrace.StatusCode {
	if httpStatusCode >= 100 && httpStatusCode < 399 {
		return ptrace.StatusCodeUnset
	}
	return ptrace.StatusCodeError
}

func jSpanKindToInternal(spanKind string) ptrace.SpanKind {
	switch spanKind {
	case "client":
		return ptrace.SpanKindClient
	case "server":
		return ptrace.SpanKindServer
	case "producer":
		return ptrace.SpanKindProducer
	case "consumer":
		return ptrace.SpanKindConsumer
	case "internal":
		return ptrace.SpanKindInternal
	}
	return ptrace.SpanKindUnspecified
}

func dbLogsToSpanEvents(logs []dbmodel.Log, events ptrace.SpanEventSlice) {
	if len(logs) == 0 {
		return
	}

	events.EnsureCapacity(len(logs))

	for i, log := range logs {
		var event ptrace.SpanEvent
		if events.Len() > i {
			event = events.At(i)
		} else {
			event = events.AppendEmpty()
		}
		//nolint:gosec // G115 // dblog.Timestamp is guaranteed non-negative (epoch microseconds) by schema constraints
		event.SetTimestamp(dbTimeStampToOTLPTimeStamp(uint64(log.Timestamp)))
		if len(log.Fields) == 0 {
			continue
		}

		attrs := event.Attributes()
		attrs.EnsureCapacity(len(log.Fields))
		dbTagsToAttributes(log.Fields, attrs)
		if name, ok := attrs.Get(eventNameAttr); ok {
			event.SetName(name.Str())
			attrs.Remove(eventNameAttr)
		}
	}
}

// dbReferencesToSpanLinks sets internal span links based on jaeger span references skipping excludeParentID
func dbReferencesToSpanLinks(refs []dbmodel.SpanRef, excludeParentID int64, spanLinks ptrace.SpanLinkSlice) {
	if len(refs) == 0 || len(refs) == 1 && refs[0].SpanID == excludeParentID && refs[0].RefType == dbmodel.ChildOf {
		return
	}

	spanLinks.EnsureCapacity(len(refs))
	for _, ref := range refs {
		if ref.SpanID == excludeParentID && ref.RefType == dbmodel.ChildOf {
			continue
		}

		link := spanLinks.AppendEmpty()
		link.SetTraceID(pcommon.TraceID(ref.TraceID))
		//nolint:gosec // G115 // bit-preserving uint64<->int64 conversion for opaque IDs
		link.SetSpanID(idutils.UInt64ToSpanID(uint64(ref.SpanID)))
		link.Attributes().PutStr(otelsemconv.AttributeOpentracingRefType, dbRefTypeToAttribute(ref.RefType))
	}
}

func getTraceStateFromAttrs(attrs pcommon.Map) string {
	traceState := ""
	// TODO Bring this inline with solution for jaegertracing/jaeger-client-java #702 once available
	if attr, ok := attrs.Get(tagW3CTraceState); ok {
		traceState = attr.Str()
		attrs.Remove(tagW3CTraceState)
	}
	return traceState
}

func dbSpanToScope(span *dbmodel.Span, scopeSpan ptrace.ScopeSpans) {
	if libraryName, ok := getAndDeleteTag(span, otelsemconv.AttributeOtelScopeName); ok {
		scopeSpan.Scope().SetName(libraryName)
		if libraryVersion, ok := getAndDeleteTag(span, otelsemconv.AttributeOtelScopeVersion); ok {
			scopeSpan.Scope().SetVersion(libraryVersion)
		}
	}
}

func getAndDeleteTag(span *dbmodel.Span, key string) (string, bool) {
	for i, tag := range span.Tags {
		if tag.Key == key {
			val := tag.ValueString
			span.Tags = append(span.Tags[:i], span.Tags[i+1:]...)
			return val, true
		}
	}
	return "", false
}

func dbRefTypeToAttribute(ref string) string {
	if ref == dbmodel.ChildOf {
		return otelsemconv.AttributeOpentracingRefTypeChildOf
	}
	return otelsemconv.AttributeOpentracingRefTypeFollowsFrom
}

// dbTimeStampToOTLPTimeStamp converts the db timestamp which is in microseconds
// to nanoseconds which is the OTLP standard.
func dbTimeStampToOTLPTimeStamp(timestamp uint64) pcommon.Timestamp {
	return pcommon.Timestamp(timestamp * 1000)
}
