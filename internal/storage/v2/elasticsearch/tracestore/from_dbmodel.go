// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/jaegerproto_to_traces.go

package tracestore

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

var errType = errors.New("invalid type")

// FromDBModel converts multiple ES DB Spans to internal traces
func FromDBModel(spans []dbmodel.Span) (ptrace.Traces, error) {
	traceData := ptrace.NewTraces()
	if len(spans) == 0 {
		return traceData, nil
	}

	resourceSpans := traceData.ResourceSpans()
	resourceSpans.EnsureCapacity(len(spans))
	err := dbSpansToSpans(spans, resourceSpans)

	return traceData, err
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
		attrs.PutStr(conventions.AttributeServiceName, serviceName)
	} else {
		attrs.EnsureCapacity(len(tags))
	}
	dbTagsToAttributes(tags, attrs)
}

func dbSpansToSpans(dbSpans []dbmodel.Span, resourceSpans ptrace.ResourceSpansSlice) error {
	for i := range dbSpans {
		span := &dbSpans[i]
		resourceSpan := resourceSpans.AppendEmpty()
		dbProcessToResource(span.Process, resourceSpan.Resource())

		scopeSpans := resourceSpan.ScopeSpans()
		scopeSpan := scopeSpans.AppendEmpty()
		dbSpanToScope(span, scopeSpan)

		// TODO there should be no errors returned from translation from db model
		err := dbSpanToSpan(span, scopeSpan.Spans().AppendEmpty())
		if err != nil {
			return err
		}
	}
	return nil
}

func dbSpanToSpan(dbSpan *dbmodel.Span, span ptrace.Span) error {
	traceId, err := fromDbTraceId(dbSpan.TraceID)
	if err != nil {
		return err
	}
	spanId, err := fromDbSpanId(dbSpan.SpanID)
	if err != nil {
		return err
	}
	if dbSpan.ParentSpanID != "" {
		parentSpanId, err := fromDbSpanId(dbSpan.ParentSpanID)
		if err != nil {
			return err
		}
		span.SetParentSpanID(parentSpanId)
	}
	span.SetTraceID(traceId)
	span.SetSpanID(spanId)
	span.SetName(dbSpan.OperationName)

	startTime := model.EpochMicrosecondsAsTime(dbSpan.StartTime)
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))

	duration := model.MicrosecondsAsDuration(dbSpan.Duration)
	endTime := startTime.Add(duration)
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(endTime))

	// TODO rewrite this to use a single loop over tags
	// and map special tag names to OTEL Span fields
	attrs := span.Attributes()
	attrs.EnsureCapacity(len(dbSpan.Tags))
	dbTagsToAttributes(dbSpan.Tags, attrs)
	if spanKindAttr, ok := attrs.Get(model.SpanKindKey); ok {
		span.SetKind(dbSpanKindToOTELSpanKind(spanKindAttr.Str()))
		attrs.Remove(model.SpanKindKey)
	}
	setSpanStatus(attrs, span)

	span.TraceState().FromRaw(getTraceStateFromAttrs(attrs))

	// drop the attributes slice if all of them were replaced during translation
	if attrs.Len() == 0 {
		attrs.Clear()
	}
	dbParentSpanId := getParentSpanId(dbSpan)
	if dbParentSpanId != "" {
		parentSpanId, err := fromDbSpanId(dbParentSpanId)
		if err != nil {
			return err
		}
		span.SetParentSpanID(parentSpanId)
	}
	dbSpanLogsToSpanEvents(dbSpan.Logs, span.Events())
	return dbSpanRefsToSpanEvents(dbSpan.References, dbParentSpanId, span.Links())
}

func dbTagsToAttributes(tags []dbmodel.KeyValue, attributes pcommon.Map) {
	for _, tag := range tags {
		tagValue, ok := tag.Value.(string)
		if !ok {
			// We have to do this as we are unsure that whether bool will be a string or a bool
			tagBoolVal, boolOk := tag.Value.(bool)
			if boolOk && tag.Type == dbmodel.BoolType {
				attributes.PutBool(tag.Key, tagBoolVal)
			} else {
				attributes.PutStr(tag.Key, "Got non string inputValue for the key "+tag.Key)
			}
			continue
		}
		switch tag.Type {
		case dbmodel.StringType:
			attributes.PutStr(tag.Key, tagValue)
		case dbmodel.BoolType:
			convBoolVal, err := strconv.ParseBool(tagValue)
			if err != nil {
				recordTagConversionError(tag, err, attributes)
			} else {
				attributes.PutBool(tag.Key, convBoolVal)
			}
		case dbmodel.Int64Type:
			intVal, err := strconv.ParseInt(tagValue, 10, 64)
			if err != nil {
				recordTagConversionError(tag, err, attributes)
			} else {
				attributes.PutInt(tag.Key, intVal)
			}
		case dbmodel.Float64Type:
			floatVal, err := strconv.ParseFloat(tagValue, 64)
			if err != nil {
				recordTagConversionError(tag, err, attributes)
			} else {
				attributes.PutDouble(tag.Key, floatVal)
			}
		case dbmodel.BinaryType:
			value, err := hex.DecodeString(tagValue)
			if err != nil {
				recordTagConversionError(tag, err, attributes)
			} else {
				attributes.PutEmptyBytes(tag.Key).FromRaw(value)
			}
		default:
			attributes.PutStr(tag.Key, fmt.Sprintf("<Unknown Jaeger TagType %q>", tag.Type))
		}
	}
}

func recordTagConversionError(kv dbmodel.KeyValue, err error, dest pcommon.Map) {
	dest.PutStr(kv.Key, fmt.Sprintf("Can't convert the type %s for the key %s: %v", string(kv.Type), kv.Key, err))
}

func setSpanStatus(attrs pcommon.Map, span ptrace.Span) {
	dest := span.Status()
	statusCode := ptrace.StatusCodeUnset
	statusMessage := ""
	statusExists := false

	if codeAttr, ok := attrs.Get(conventions.OtelStatusCode); ok {
		if !statusExists {
			// The error tag is the ultimate truth for a Jaeger spans' error
			// status. Only parse the otel.status_code tag if the error tag is
			// not set to true.
			statusExists = true
			switch strings.ToUpper(codeAttr.Str()) {
			case statusOk:
				statusCode = ptrace.StatusCodeOk
			case statusError:
				statusCode = ptrace.StatusCodeError
			}

			if desc, ok := extractStatusDescFromAttr(attrs); ok {
				statusMessage = desc
			}
		}
		// Regardless of error tag inputValue, remove the otel.status_code tag. The
		// otel.status_message tag will have already been removed if
		// statusExists is true.
		attrs.Remove(conventions.OtelStatusCode)
	} else if httpCodeAttr, ok := attrs.Get(conventions.AttributeHTTPStatusCode); !statusExists && ok {
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
	if msgAttr, ok := attrs.Get(conventions.OtelStatusDescription); ok {
		msg := msgAttr.Str()
		attrs.Remove(conventions.OtelStatusDescription)
		return msg, true
	}
	return "", false
}

// codeFromAttr returns the integer code inputValue from attrVal. An error is
// returned if the code is not represented by an integer or string inputValue in
// the attrVal or the inputValue is outside the bounds of an int representation.
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

func dbSpanKindToOTELSpanKind(spanKind string) ptrace.SpanKind {
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

func dbSpanLogsToSpanEvents(logs []dbmodel.Log, events ptrace.SpanEventSlice) {
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

		event.SetTimestamp(pcommon.NewTimestampFromTime(model.EpochMicrosecondsAsTime(log.Timestamp)))
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

// dbSpanRefsToSpanEvents sets internal span links based on db references skipping excludeParentID
func dbSpanRefsToSpanEvents(refs []dbmodel.Reference, excludeParentID dbmodel.SpanID, spanLinks ptrace.SpanLinkSlice) error {
	if len(refs) == 0 || len(refs) == 1 && refs[0].SpanID == excludeParentID && refs[0].RefType == dbmodel.ChildOf {
		return nil
	}

	spanLinks.EnsureCapacity(len(refs))
	for _, ref := range refs {
		if ref.SpanID == excludeParentID && ref.RefType == dbmodel.ChildOf {
			continue
		}

		link := spanLinks.AppendEmpty()
		refTraceId, err := fromDbTraceId(ref.TraceID)
		if err != nil {
			return err
		}
		refSpanId, err := fromDbSpanId(ref.SpanID)
		if err != nil {
			return err
		}
		link.SetTraceID(refTraceId)
		link.SetSpanID(refSpanId)
		link.Attributes().PutStr(conventions.AttributeOpentracingRefType, dbRefTypeToAttribute(ref.RefType))
	}
	return nil
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
	if libraryName, ok := getAndDeleteTag(span, conventions.AttributeOtelScopeName); ok {
		scopeSpan.Scope().SetName(libraryName)
		if libraryVersion, ok := getAndDeleteTag(span, conventions.AttributeOtelScopeVersion); ok {
			scopeSpan.Scope().SetVersion(libraryVersion)
		}
	}
}

func getAndDeleteTag(span *dbmodel.Span, key string) (string, bool) {
	for i := range span.Tags {
		if span.Tags[i].Key == key {
			if val, ok := span.Tags[i].Value.(string); ok {
				span.Tags = append(span.Tags[:i], span.Tags[i+1:]...)
				return val, true
			}
		}
	}
	return "", false
}

func dbRefTypeToAttribute(ref dbmodel.ReferenceType) string {
	if ref == dbmodel.ChildOf {
		return conventions.AttributeOpentracingRefTypeChildOf
	}
	return conventions.AttributeOpentracingRefTypeFollowsFrom
}
