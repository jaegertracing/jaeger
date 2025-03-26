// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/traces_to_jaegerproto.go

package tracestore

import (
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
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

// ProtoFromTraces translates internal trace data into the Jaeger Proto for GRPC.
// Returns slice of translated Jaeger batches and error if translation failed.
func ProtoFromTraces(td ptrace.Traces) []*dbmodel.Span {
	resourceSpans := td.ResourceSpans()

	if resourceSpans.Len() == 0 {
		return nil
	}

	batches := make([]*dbmodel.Span, 0, resourceSpans.Len())
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		batch := resourceSpansToJaegerProto(rs)
		if batch != nil {
			batches = append(batches, batch...)
		}
	}

	return batches
}

func resourceSpansToJaegerProto(rs ptrace.ResourceSpans) []*dbmodel.Span {
	resource := rs.Resource()
	ilss := rs.ScopeSpans()

	if resource.Attributes().Len() == 0 && ilss.Len() == 0 {
		return nil
	}

	process := resourceToJaegerProtoProcess(resource)

	if ilss.Len() == 0 {
		return []*dbmodel.Span{}
	}

	// Approximate the number of the spans as the number of the spans in the first
	// instrumentation library info.
	jSpans := make([]*dbmodel.Span, 0, ilss.At(0).Spans().Len())

	for i := 0; i < ilss.Len(); i++ {
		ils := ilss.At(i)
		spans := ils.Spans()
		for j := 0; j < spans.Len(); j++ {
			span := spans.At(j)
			jSpan := spanToJaegerProto(span, ils.Scope(), *process)
			if jSpan != nil {
				jSpans = append(jSpans, jSpan)
			}
		}
	}

	return jSpans
}

func resourceToJaegerProtoProcess(resource pcommon.Resource) *dbmodel.Process {
	process := &dbmodel.Process{}
	attrs := resource.Attributes()
	if attrs.Len() == 0 {
		process.ServiceName = noServiceName
		return process
	}
	attrsCount := attrs.Len()
	if serviceName, ok := attrs.Get(conventions.AttributeServiceName); ok {
		process.ServiceName = serviceName.Str()
		attrsCount--
	}
	if attrsCount == 0 {
		return process
	}

	tags := make([]dbmodel.KeyValue, 0, attrsCount)
	process.Tags = appendTagsFromResourceAttributes(tags, attrs)
	return process
}

func appendTagsFromResourceAttributes(dest []dbmodel.KeyValue, attrs pcommon.Map) []dbmodel.KeyValue {
	if attrs.Len() == 0 {
		return dest
	}

	for key, attr := range attrs.All() {
		if key == conventions.AttributeServiceName {
			continue
		}
		dest = append(dest, attributeToJaegerProtoTag(key, attr))
	}
	return dest
}

func appendTagsFromAttributes(dest []dbmodel.KeyValue, attrs pcommon.Map) []dbmodel.KeyValue {
	if attrs.Len() == 0 {
		return dest
	}
	for key, attr := range attrs.All() {
		dest = append(dest, attributeToJaegerProtoTag(key, attr))
	}
	return dest
}

func attributeToJaegerProtoTag(key string, attr pcommon.Value) dbmodel.KeyValue {
	tag := dbmodel.KeyValue{Key: key, Value: attr.AsString()}
	switch attr.Type() {
	case pcommon.ValueTypeStr:
		tag.Type = dbmodel.StringType
	case pcommon.ValueTypeInt:
		tag.Type = dbmodel.Int64Type
	case pcommon.ValueTypeBool:
		tag.Type = dbmodel.BoolType
	case pcommon.ValueTypeDouble:
		tag.Type = dbmodel.Float64Type
	case pcommon.ValueTypeBytes:
		tag.Type = dbmodel.BinaryType
	case pcommon.ValueTypeMap, pcommon.ValueTypeSlice:
		tag.Type = dbmodel.StringType
	}
	return tag
}

func spanToJaegerProto(span ptrace.Span, libraryTags pcommon.InstrumentationScope, process dbmodel.Process) *dbmodel.Span {
	traceID := dbmodel.TraceID(span.TraceID().String())
	parentSpanID := dbmodel.SpanID(span.ParentSpanID().String())
	jReferences := makeJaegerProtoReferences(span.Links(), parentSpanID, traceID)

	startTime := span.StartTimestamp().AsTime()
	return &dbmodel.Span{
		TraceID:       traceID,
		SpanID:        dbmodel.SpanID(span.SpanID().String()),
		OperationName: span.Name(),
		References:    jReferences,
		StartTime:     model.TimeAsEpochMicroseconds(startTime),
		Duration:      model.DurationAsMicroseconds(span.EndTimestamp().AsTime().Sub(startTime)),
		Tags:          getJaegerProtoSpanTags(span, libraryTags),
		Logs:          spanEventsToJaegerProtoLogs(span.Events()),
		Process:       process,
	}
}

func getJaegerProtoSpanTags(span ptrace.Span, scope pcommon.InstrumentationScope) []dbmodel.KeyValue {
	var spanKindTag, statusCodeTag, errorTag, statusMsgTag dbmodel.KeyValue
	var spanKindTagFound, statusCodeTagFound, errorTagFound, statusMsgTagFound bool

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

	errorTag, errorTagFound = getErrorTagFromStatusCode(status.Code())
	if errorTagFound {
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
	if errorTagFound {
		tags = append(tags, errorTag)
	}
	if statusMsgTagFound {
		tags = append(tags, statusMsgTag)
	}
	if traceStateTagsFound {
		tags = append(tags, traceStateTags...)
	}
	return tags
}

// makeJaegerProtoReferences constructs jaeger span references based on parent span ID and span links.
// The parent span ID is used to add a CHILD_OF reference, _unless_ it is referenced from one of the links.
func makeJaegerProtoReferences(links ptrace.SpanLinkSlice, parentSpanID dbmodel.SpanID, traceID dbmodel.TraceID) []dbmodel.Reference {
	refsCount := links.Len()
	if parentSpanID != "" {
		refsCount++
	}

	if refsCount == 0 {
		return nil
	}

	refs := make([]dbmodel.Reference, 0, refsCount)

	// Put parent span ID at the first place because usually backends look for it
	// as the first CHILD_OF item in the model.SpanRef slice.
	if parentSpanID != "" {
		refs = append(refs, dbmodel.Reference{
			TraceID: traceID,
			SpanID:  parentSpanID,
			RefType: dbmodel.ChildOf,
		})
	}

	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		linkTraceID := dbmodel.TraceID(link.TraceID().String())
		linkSpanID := dbmodel.SpanID(link.SpanID().String())
		linkRefType := refTypeFromLink(link)
		if parentSpanID != "" && linkTraceID == traceID && linkSpanID == parentSpanID {
			// We already added a reference to this span, but maybe with the wrong type, so override.
			refs[0].RefType = linkRefType
			continue
		}
		refs = append(refs, dbmodel.Reference{
			TraceID: linkTraceID,
			SpanID:  linkSpanID,
			RefType: refTypeFromLink(link),
		})
	}

	return refs
}

func spanEventsToJaegerProtoLogs(events ptrace.SpanEventSlice) []dbmodel.Log {
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
				Key:   eventNameAttr,
				Type:  dbmodel.StringType,
				Value: event.Name(),
			})
		}
		fields = appendTagsFromAttributes(fields, event.Attributes())
		logs = append(logs, dbmodel.Log{
			Timestamp: model.TimeAsEpochMicroseconds(event.Timestamp().AsTime()),
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
		Key:   model.SpanKindKey,
		Type:  dbmodel.StringType,
		Value: tagStr,
	}, true
}

func getTagFromStatusCode(statusCode ptrace.StatusCode) (dbmodel.KeyValue, bool) {
	switch statusCode {
	case ptrace.StatusCodeError:
		return dbmodel.KeyValue{
			Key:   conventions.OtelStatusCode,
			Type:  dbmodel.StringType,
			Value: statusError,
		}, true
	case ptrace.StatusCodeOk:
		return dbmodel.KeyValue{
			Key:   conventions.OtelStatusCode,
			Type:  dbmodel.StringType,
			Value: statusOk,
		}, true
	}
	return dbmodel.KeyValue{}, false
}

func getErrorTagFromStatusCode(statusCode ptrace.StatusCode) (dbmodel.KeyValue, bool) {
	if statusCode == ptrace.StatusCodeError {
		return dbmodel.KeyValue{
			Key:   tagError,
			Value: true,
			Type:  dbmodel.BoolType,
		}, true
	}
	return dbmodel.KeyValue{}, false
}

func getTagFromStatusMsg(statusMsg string) (dbmodel.KeyValue, bool) {
	if statusMsg == "" {
		return dbmodel.KeyValue{}, false
	}
	return dbmodel.KeyValue{
		Key:   conventions.OtelStatusDescription,
		Value: statusMsg,
		Type:  dbmodel.StringType,
	}, true
}

func getTagsFromTraceState(traceState string) ([]dbmodel.KeyValue, bool) {
	var keyValues []dbmodel.KeyValue
	exists := traceState != ""
	if exists {
		// TODO Bring this inline with solution for jaegertracing/jaeger-client-java #702 once available
		kv := dbmodel.KeyValue{
			Key:   tagW3CTraceState,
			Value: traceState,
			Type:  dbmodel.StringType,
		}
		keyValues = append(keyValues, kv)
	}
	return keyValues, exists
}

func getTagsFromInstrumentationLibrary(il pcommon.InstrumentationScope) ([]dbmodel.KeyValue, bool) {
	var keyValues []dbmodel.KeyValue
	if ilName := il.Name(); ilName != "" {
		kv := dbmodel.KeyValue{
			Key:   conventions.AttributeOtelScopeName,
			Value: ilName,
			Type:  dbmodel.StringType,
		}
		keyValues = append(keyValues, kv)
	}
	if ilVersion := il.Version(); ilVersion != "" {
		kv := dbmodel.KeyValue{
			Key:   conventions.AttributeOtelScopeVersion,
			Value: ilVersion,
			Type:  dbmodel.StringType,
		}
		keyValues = append(keyValues, kv)
	}

	return keyValues, true
}

func refTypeFromLink(link ptrace.SpanLink) dbmodel.ReferenceType {
	refTypeAttr, ok := link.Attributes().Get(conventions.AttributeOpentracingRefType)
	if !ok {
		return dbmodel.FollowsFrom
	}
	return strToJRefType(refTypeAttr.Str())
}

func strToJRefType(attr string) dbmodel.ReferenceType {
	if attr == conventions.AttributeOpentracingRefTypeChildOf {
		return dbmodel.ChildOf
	}
	// There are only 2 types of SpanRefType we assume that everything
	// that's not a model.ChildOf is a model.FollowsFrom
	return dbmodel.FollowsFrom
}
