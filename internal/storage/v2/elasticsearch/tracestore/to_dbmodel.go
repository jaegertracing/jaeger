// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/traces_to_jaegerproto.go

package tracestore

import (
	"encoding/hex"
	"math"

	"go.opentelemetry.io/collector/featuregate"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	conventions "github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

const (
	noServiceName    = "OTLPResourceNoServiceName"
	eventNameAttr    = "event"
	statusError      = "ERROR"
	statusOk         = "OK"
	tagW3CTraceState = "w3c.tracestate"
	tagError         = "error"
)

var _ = featuregate.GlobalRegistry().MustRegister(
	"jaeger.es.omitParentSpanIDReference",
	featuregate.StageStable,
	featuregate.WithRegisterFromVersion("v2.20.0"),
	featuregate.WithRegisterToVersion("v2.26.0"),
	featuregate.WithRegisterDescription(
		"The Elasticsearch/OpenSearch write path does not encode the parent span ID as a "+
			"synthetic CHILD_OF reference; the parent is written only to the dedicated "+
			"parentSpanID field.",
	),
)

// ToDBModel translates internal trace data into a slice of DB Spans.
func ToDBModel(td ptrace.Traces) []dbmodel.Span {
	resourceSpans := td.ResourceSpans()

	if resourceSpans.Len() == 0 {
		return nil
	}

	batches := make([]dbmodel.Span, 0, resourceSpans.Len())
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		batch := resourceSpansToDbSpans(rs)
		if batch != nil {
			batches = append(batches, batch...)
		}
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
	if attrs.Len() == 0 {
		process.ServiceName = noServiceName
		return process
	}
	tags := make([]dbmodel.KeyValue, 0, attrs.Len())
	for key, attr := range attrs.All() {
		if key == conventions.ServiceNameKey {
			process.ServiceName = attr.AsString()
			continue
		}
		tags = append(tags, attributeToDbTag(key, attr))
	}
	process.Tags = tags
	return process
}

func appendTagsFromAttributes(dest []dbmodel.KeyValue, attrs pcommon.Map) []dbmodel.KeyValue {
	for key, attr := range attrs.All() {
		dest = append(dest, attributeToDbTag(key, attr))
	}
	return dest
}

func attributeToDbTag(key string, attr pcommon.Value) dbmodel.KeyValue {
	var tag dbmodel.KeyValue
	switch attr.Type() {
	case pcommon.ValueTypeBytes:
		tag = dbmodel.KeyValue{Key: key, Value: hex.EncodeToString(attr.Bytes().AsRaw())}
	case pcommon.ValueTypeMap, pcommon.ValueTypeSlice:
		tag = dbmodel.KeyValue{Key: key, Value: attr.AsString()}
	case pcommon.ValueTypeDouble:
		tag = dbmodel.KeyValue{Key: key, Value: doubleToDbValue(attr.Double())}
	default:
		tag = dbmodel.KeyValue{Key: key, Value: attr.AsRaw()}
	}
	switch attr.Type() {
	case pcommon.ValueTypeInt:
		tag.Type = dbmodel.Int64Type
	case pcommon.ValueTypeBool:
		tag.Type = dbmodel.BoolType
	case pcommon.ValueTypeDouble:
		tag.Type = dbmodel.Float64Type
	case pcommon.ValueTypeBytes:
		tag.Type = dbmodel.BinaryType
	default:
		tag.Type = dbmodel.StringType
	}
	return tag
}

// doubleToDbValue returns a finite double unchanged. NaN and the infinities
// become the OTLP/JSON tokens "NaN", "Infinity", and "-Infinity", because
// encoding/json rejects the bare floats and would drop the whole span. The
// tag keeps Float64Type, and FromDBModel parses the token back into a double.
func doubleToDbValue(f float64) any {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	default:
		return f
	}
}

func spanToDbSpan(span ptrace.Span, libraryTags pcommon.InstrumentationScope, process dbmodel.Process) dbmodel.Span {
	startTime := span.StartTimestamp().AsTime()
	return dbmodel.Span{
		TraceID:         dbmodel.TraceID(span.TraceID().String()),
		SpanID:          dbmodel.SpanID(span.SpanID().String()),
		ParentSpanID:    dbmodel.SpanID(span.ParentSpanID().String()),
		OperationName:   span.Name(),
		References:      linksToDbSpanRefs(span.Links()),
		StartTime:       model.TimeAsEpochMicroseconds(startTime),
		StartTimeMillis: model.TimeAsEpochMicroseconds(startTime) / 1000,
		Duration:        model.DurationAsMicroseconds(span.EndTimestamp().AsTime().Sub(startTime)),
		Tags:            getDbSpanTags(span, libraryTags),
		Logs:            spanEventsToDbSpanLogs(span.Events()),
		Process:         process,
		Flags:           span.Flags(),
	}
}

func getDbSpanTags(span ptrace.Span, scope pcommon.InstrumentationScope) []dbmodel.KeyValue {
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

// linksToDbSpanRefs converts span links into jaeger span references. It adds no reference
// for the parent span ID, which is stored in the parentSpanID field of the span.
func linksToDbSpanRefs(links ptrace.SpanLinkSlice) []dbmodel.Reference {
	if links.Len() == 0 {
		return nil
	}

	refs := make([]dbmodel.Reference, 0, links.Len())
	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		refs = append(refs, dbmodel.Reference{
			TraceID: dbmodel.TraceID(link.TraceID().String()),
			SpanID:  dbmodel.SpanID(link.SpanID().String()),
			RefType: refTypeFromLink(link),
		})
	}

	return refs
}

func spanEventsToDbSpanLogs(events ptrace.SpanEventSlice) []dbmodel.Log {
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
	case ptrace.StatusCodeOk:
		return dbmodel.KeyValue{
			Key:   conventions.OtelStatusCode,
			Type:  dbmodel.StringType,
			Value: statusOk,
		}, true
	case ptrace.StatusCodeError:
		// For backward compatibility, we also include the error tag
		// which was previously used in the test fixtures
		return dbmodel.KeyValue{
			Key:   "error",
			Type:  dbmodel.BoolType,
			Value: true,
		}, true
	default:
		return dbmodel.KeyValue{}, false
	}
}

func getTagFromStatusMsg(statusMsg string) (dbmodel.KeyValue, bool) {
	if statusMsg == "" {
		return dbmodel.KeyValue{}, false
	}
	return dbmodel.KeyValue{
		Key:   conventions.OtelStatusDescription,
		Type:  dbmodel.StringType,
		Value: statusMsg,
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
			Type:  dbmodel.StringType,
			Value: ilName,
		}
		keyValues = append(keyValues, kv)
	}
	if ilVersion := il.Version(); ilVersion != "" {
		kv := dbmodel.KeyValue{
			Key:   conventions.AttributeOtelScopeVersion,
			Type:  dbmodel.StringType,
			Value: ilVersion,
		}
		keyValues = append(keyValues, kv)
	}
	return keyValues, len(keyValues) > 0
}

func refTypeFromLink(link ptrace.SpanLink) dbmodel.ReferenceType {
	refTypeAttr, ok := link.Attributes().Get(conventions.AttributeOpentracingRefType)
	if !ok {
		return dbmodel.FollowsFrom
	}
	return strToDbSpanRefType(refTypeAttr.Str())
}

func strToDbSpanRefType(attr string) dbmodel.ReferenceType {
	if attr == conventions.AttributeOpentracingRefTypeChildOf {
		return dbmodel.ChildOf
	}
	// There are only 2 types of SpanRefType we assume that everything
	// that's not a model.ChildOf is a model.FollowsFrom
	return dbmodel.FollowsFrom
}
