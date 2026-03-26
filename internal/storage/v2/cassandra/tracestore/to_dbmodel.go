// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/traces_to_jaegerproto.go

package tracestore

import (
	"bytes"
	"encoding/binary"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

const (
	noServiceName    = "OTLPResourceNoServiceName"
	eventNameAttr    = "event"
	statusError      = "ERROR"
	statusOk         = "OK"
	tagError         = "error"
	tagW3CTraceState = "w3c.tracestate"
	tagHTTPStatusMsg = "http.status_message"
)

// ToDBModel translates internal trace data into the DB Spans.
// Returns a slice of translated DB Spans.
func ToDBModel(td ptrace.Traces) []dbmodel.Span {
	resourceSpans := td.ResourceSpans()

	if resourceSpans.Len() == 0 {
		return []dbmodel.Span{}
	}

	batches := make([]dbmodel.Span, 0, td.SpanCount())
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		batch := resourceSpansToDbSpans(rs)
		batches = append(batches, batch...)
	}

	return batches
}

func resourceSpansToDbSpans(resourceSpans ptrace.ResourceSpans) []dbmodel.Span {
	resource := resourceSpans.Resource()
	scopeSpans := resourceSpans.ScopeSpans()

	if scopeSpans.Len() == 0 {
		return []dbmodel.Span{}
	}

	process := resourceToDbProcess(resource)

	// Approximate the number of the spans as the number of the spans in the first
	// instrumentation library info.
	dbSpans := make([]dbmodel.Span, 0, scopeSpans.At(0).Spans().Len())

	for _, scopeSpan := range scopeSpans.All() {
		for _, span := range scopeSpan.Spans().All() {
			dbSpan := spanToDbSpan(span, scopeSpan.Scope(), process)
			dbSpans = append(dbSpans, dbSpan)
		}
	}

	return dbSpans
}

func resourceToDbProcess(resource pcommon.Resource) dbmodel.Process {
	process := dbmodel.Process{}
	attrs := resource.Attributes()
	process.ServiceName = noServiceName
	if attrs.Len() == 0 {
		return process
	}
	tags := make([]dbmodel.KeyValue, 0, attrs.Len())
	for key, attr := range attrs.All() {
		if key == otelsemconv.ServiceNameKey {
			process.ServiceName = attr.AsString()
			continue
		}
		tags = append(tags, attributeToDbTag(key, attr))
	}
	process.Tags = tags
	return process
}

func appendTagsFromAttributes(tags []dbmodel.KeyValue, attrs pcommon.Map) []dbmodel.KeyValue {
	if attrs.Len() == 0 {
		return tags
	}
	for key, attr := range attrs.All() {
		tags = append(tags, attributeToDbTag(key, attr))
	}
	return tags
}

func attributeToDbTag(key string, attr pcommon.Value) dbmodel.KeyValue {
	tag := dbmodel.KeyValue{Key: key}
	switch attr.Type() {
	case pcommon.ValueTypeInt:
		tag.ValueType = dbmodel.Int64Type
		tag.ValueInt64 = attr.Int()
	case pcommon.ValueTypeBool:
		tag.ValueType = dbmodel.BoolType
		tag.ValueBool = attr.Bool()
	case pcommon.ValueTypeDouble:
		tag.ValueType = dbmodel.Float64Type
		tag.ValueFloat64 = attr.Double()
	case pcommon.ValueTypeBytes:
		tag.ValueType = dbmodel.BinaryType
		tag.ValueBinary = attr.Bytes().AsRaw()
	case pcommon.ValueTypeMap, pcommon.ValueTypeSlice:
		tag.ValueType = dbmodel.StringType
		tag.ValueString = attr.AsString()
	default:
		tag.ValueType = dbmodel.StringType
		tag.ValueString = attr.Str()
	}
	return tag
}

func spanToDbSpan(span ptrace.Span, scope pcommon.InstrumentationScope, process dbmodel.Process) dbmodel.Span {
	dbTraceId := dbmodel.TraceID(span.TraceID())
	dbReferences := linksToDbSpanRefs(span.Links(), spanIDToDbSpanId(span.ParentSpanID()), dbTraceId)
	startTime := span.StartTimestamp().AsTime()
	return dbmodel.Span{
		TraceID:       dbTraceId,
		SpanID:        spanIDToDbSpanId(span.SpanID()),
		OperationName: span.Name(),
		Refs:          dbReferences,
		//nolint:gosec // G115 // OTLP timestamp is nanoseconds since epoch (semantically non-negative), safe to convert to int64 microseconds
		StartTime: int64(model.TimeAsEpochMicroseconds(startTime)),
		//nolint:gosec // G115 // span.EndTime - span.StartTime is guaranteed non-negative by schema constraints
		Duration: int64(model.DurationAsMicroseconds(span.EndTimestamp().AsTime().Sub(startTime))),
		Tags:     getDbTags(span, scope),
		Logs:     spanEventsToDbLogs(span.Events()),
		Process:  process,
		//nolint:gosec // G115 // span.Flags is uint32, converting to int32 for DB storage (semantically non-negative, fits in int32)
		Flags:       int32(span.Flags()),
		ServiceName: process.ServiceName,
		ParentID:    spanIDToDbSpanId(span.ParentSpanID()),
	}
}

func getDbTags(span ptrace.Span, scope pcommon.InstrumentationScope) []dbmodel.KeyValue {
	var spanKindTag, statusCodeTag, statusMsgTag dbmodel.KeyValue
	var spanKindTagFound, statusCodeTagFound, statusMsgTagFound bool

	libraryTags, libraryTagsFound := getTagsFromInstrumentationLibrary(scope)

	tagsCount := span.Attributes().Len() + len(libraryTags)

	spanKindTag, spanKindTagFound = getTagFromSpanKind(span.Kind())
	if spanKindTagFound {
		tagsCount++
	}
	status := span.Status()
	statusCodeTag, statusCodeTagFound = getTagFromStatusCode(status.Code())
	if statusCodeTagFound {
		tagsCount++
	}

	statusMsgTag, statusMsgTagFound = getTagFromStatusMsg(status.Message())
	if statusMsgTagFound {
		tagsCount++
	}

	traceStateTags, traceStateTagsFound := getTagsFromTraceState(span.TraceState().AsRaw())
	if traceStateTagsFound {
		tagsCount += len(traceStateTags)
	}

	if tagsCount == 0 {
		return nil
	}

	tags := make([]dbmodel.KeyValue, 0, tagsCount)
	if libraryTagsFound {
		tags = append(tags, libraryTags...)
	}
	tags = appendTagsFromAttributes(tags, span.Attributes())
	if spanKindTagFound {
		tags = append(tags, spanKindTag)
	}
	if statusCodeTagFound {
		tags = append(tags, statusCodeTag)
	}
	if statusMsgTagFound {
		tags = append(tags, statusMsgTag)
	}
	if traceStateTagsFound {
		tags = append(tags, traceStateTags...)
	}
	return tags
}

func spanIDToDbSpanId(spanID pcommon.SpanID) int64 {
	//nolint:gosec // G115 // bit-preserving conversion between uint64 and int64 for opaque SpanID
	return int64(binary.BigEndian.Uint64(spanID[:]))
}

// linksToDbSpanRefs constructs jaeger span references based on parent span ID and span links.
// The parent span ID is used to add a CHILD_OF reference, _unless_ it is referenced from one of the links.
func linksToDbSpanRefs(links ptrace.SpanLinkSlice, parentSpanID int64, traceID dbmodel.TraceID) []dbmodel.SpanRef {
	refsCount := links.Len()
	if parentSpanID != 0 {
		refsCount++
	}

	if refsCount == 0 {
		return nil
	}

	refs := make([]dbmodel.SpanRef, 0, refsCount)

	// Put parent span ID at the first place because usually backends look for it
	// as the first CHILD_OF item in the model.SpanRef slice.
	if parentSpanID != 0 {
		refs = append(refs, dbmodel.SpanRef{
			TraceID: traceID,
			SpanID:  parentSpanID,
			RefType: dbmodel.ChildOf,
		})
	}

	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		linkTraceID := dbmodel.TraceID(link.TraceID())
		linkSpanID := spanIDToDbSpanId(link.SpanID())
		linkRefType := dbRefTypeFromLink(link)
		if parentSpanID != 0 && bytes.Equal(linkTraceID[:], traceID[:]) && linkSpanID == parentSpanID {
			// We already added a reference to this span, but maybe with the wrong type, so override.
			refs[0].RefType = linkRefType
			continue
		}
		refs = append(refs, dbmodel.SpanRef{
			TraceID: linkTraceID,
			SpanID:  linkSpanID,
			RefType: linkRefType,
		})
	}

	return refs
}

func spanEventsToDbLogs(events ptrace.SpanEventSlice) []dbmodel.Log {
	if events.Len() == 0 {
		return nil
	}

	logs := make([]dbmodel.Log, 0, events.Len())
	for i := 0; i < events.Len(); i++ {
		event := events.At(i)
		fields := make([]dbmodel.KeyValue, 0, event.Attributes().Len()+1)
		_, eventAttrFound := event.Attributes().Get(eventNameAttr)
		if event.Name() != "" && !eventAttrFound {
			fields = append(fields, dbmodel.KeyValue{
				Key:         eventNameAttr,
				ValueType:   dbmodel.StringType,
				ValueString: event.Name(),
			})
		}
		fields = appendTagsFromAttributes(fields, event.Attributes())
		logs = append(logs, dbmodel.Log{
			//nolint:gosec // G115 // Timestamp is guaranteed non-negative by schema constraints
			Timestamp: int64(model.TimeAsEpochMicroseconds(event.Timestamp().AsTime())),
			Fields:    fields,
		})
	}

	return logs
}

func getTagFromSpanKind(spanKind ptrace.SpanKind) (dbmodel.KeyValue, bool) {
	var tagStr string
	switch spanKind {
	case ptrace.SpanKindClient:
		tagStr = string(model.SpanKindClient)
	case ptrace.SpanKindServer:
		tagStr = string(model.SpanKindServer)
	case ptrace.SpanKindProducer:
		tagStr = string(model.SpanKindProducer)
	case ptrace.SpanKindConsumer:
		tagStr = string(model.SpanKindConsumer)
	case ptrace.SpanKindInternal:
		tagStr = string(model.SpanKindInternal)
	default:
		return dbmodel.KeyValue{}, false
	}

	return dbmodel.KeyValue{
		Key:         model.SpanKindKey,
		ValueType:   dbmodel.StringType,
		ValueString: tagStr,
	}, true
}

func getTagFromStatusCode(statusCode ptrace.StatusCode) (dbmodel.KeyValue, bool) {
	switch statusCode {
	// For backward compatibility, we also include the error tag
	// which was previously used in the test fixtures
	case ptrace.StatusCodeError:
		return dbmodel.KeyValue{
			Key:       tagError,
			ValueType: dbmodel.BoolType,
			ValueBool: true,
		}, true
	case ptrace.StatusCodeOk:
		return dbmodel.KeyValue{
			Key:         otelsemconv.OtelStatusCode,
			ValueType:   dbmodel.StringType,
			ValueString: statusOk,
		}, true
	}
	return dbmodel.KeyValue{}, false
}

func getTagFromStatusMsg(statusMsg string) (dbmodel.KeyValue, bool) {
	if statusMsg == "" {
		return dbmodel.KeyValue{}, false
	}
	return dbmodel.KeyValue{
		Key:         otelsemconv.OtelStatusDescription,
		ValueString: statusMsg,
		ValueType:   dbmodel.StringType,
	}, true
}

func getTagsFromTraceState(traceState string) ([]dbmodel.KeyValue, bool) {
	var keyValues []dbmodel.KeyValue
	exists := traceState != ""
	if exists {
		// TODO Bring this inline with solution for jaegertracing/jaeger-client-java #702 once available
		kv := dbmodel.KeyValue{
			Key:         tagW3CTraceState,
			ValueString: traceState,
			ValueType:   dbmodel.StringType,
		}
		keyValues = append(keyValues, kv)
	}
	return keyValues, exists
}

func getTagsFromInstrumentationLibrary(scope pcommon.InstrumentationScope) ([]dbmodel.KeyValue, bool) {
	var keyValues []dbmodel.KeyValue
	if ilName := scope.Name(); ilName != "" {
		kv := dbmodel.KeyValue{
			Key:         otelsemconv.AttributeOtelScopeName,
			ValueString: ilName,
			ValueType:   dbmodel.StringType,
		}
		keyValues = append(keyValues, kv)
	}
	if ilVersion := scope.Version(); ilVersion != "" {
		kv := dbmodel.KeyValue{
			Key:         otelsemconv.AttributeOtelScopeVersion,
			ValueString: ilVersion,
			ValueType:   dbmodel.StringType,
		}
		keyValues = append(keyValues, kv)
	}

	return keyValues, len(keyValues) > 0
}

func dbRefTypeFromLink(link ptrace.SpanLink) string {
	refTypeAttr, ok := link.Attributes().Get(otelsemconv.AttributeOpentracingRefType)
	if !ok {
		return dbmodel.FollowsFrom
	}
	attr := refTypeAttr.Str()
	if attr == otelsemconv.AttributeOpentracingRefTypeChildOf {
		return dbmodel.ChildOf
	}
	// There are only 2 types of SpanRefType we assume that everything
	// that's not a model.ChildOf is a model.FollowsFrom
	return dbmodel.FollowsFrom
}
