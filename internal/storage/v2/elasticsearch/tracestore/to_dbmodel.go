// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

const (
	noServiceName    = "OTLPResourceNoServiceName"
	eventNameAttr    = "event"
	statusError      = "ERROR"
	statusOk         = "OK"
	tagError         = "error"
	tagW3CTraceState = "w3c.tracestate"
)

// ToDBModel is used to convert OTEL Traces to db spans
type ToDBModel struct {
	allTagsAsFields   bool
	tagKeysAsFields   map[string]bool
	tagDotReplacement string
}

// NewToDBModel creates NewToDBModel used to convert OTEL Traces to db spans
func NewToDBModel(allTagsAsObject bool, tagKeysAsFields []string, tagDotReplacement string) ToDBModel {
	tags := map[string]bool{}
	for _, k := range tagKeysAsFields {
		tags[k] = true
	}
	return ToDBModel{allTagsAsFields: allTagsAsObject, tagKeysAsFields: tags, tagDotReplacement: tagDotReplacement}
}

func (t ToDBModel) ConvertToDBSpans(td ptrace.Traces) []*dbmodel.Span {
	resourceSpans := td.ResourceSpans()
	if resourceSpans.Len() == 0 {
		return nil
	}
	spans := make([]*dbmodel.Span, 0, resourceSpans.Len())
	for i := 0; i < resourceSpans.Len(); i++ {
		resourceSpan := resourceSpans.At(i)
		spans = append(spans, t.resourceSpansToDbSpans(resourceSpan)...)
	}
	return spans
}

func (t ToDBModel) resourceSpansToDbSpans(rs ptrace.ResourceSpans) []*dbmodel.Span {
	resource := rs.Resource()
	ilss := rs.ScopeSpans()
	if resource.Attributes().Len() == 0 && ilss.Len() == 0 {
		return nil
	}
	process := t.resourceToDbProcess(resource)
	if ilss.Len() == 0 {
		return []*dbmodel.Span{}
	}
	jSpans := make([]*dbmodel.Span, 0, ilss.At(0).Spans().Len())
	for i := 0; i < ilss.Len(); i++ {
		ils := ilss.At(i)
		spans := ils.Spans()
		for j := 0; j < spans.Len(); j++ {
			span := spans.At(j)
			jSpan := t.spanToDbSpan(span, ils.Scope())
			jSpan.Process = *process
			if jSpan != nil {
				jSpans = append(jSpans, jSpan)
			}
		}
	}
	return jSpans
}

func (t ToDBModel) spanToDbSpan(span ptrace.Span, libraryTags pcommon.InstrumentationScope) *dbmodel.Span {
	traceId := span.TraceID()
	startTime := span.StartTimestamp().AsTime()
	tags, tagsMap := t.toDbTags(span, libraryTags)
	return &dbmodel.Span{
		TraceID:         dbmodel.TraceID(traceId.String()),
		SpanID:          dbmodel.SpanID(span.SpanID().String()),
		OperationName:   span.Name(),
		References:      makeDBReferences(span.Links(), span.ParentSpanID(), traceId),
		StartTime:       model.TimeAsEpochMicroseconds(startTime),
		StartTimeMillis: model.TimeAsEpochMicroseconds(startTime) / 1000,
		Duration:        model.DurationAsMicroseconds(span.EndTimestamp().AsTime().Sub(startTime)),
		Logs:            spanEventsToDbLogs(span.Events()),
		Tags:            tags,
		Tag:             tagsMap,
	}
}

func (t ToDBModel) toDbTags(span ptrace.Span, scope pcommon.InstrumentationScope) ([]dbmodel.KeyValue, map[string]any) {
	var tags []dbmodel.KeyValue
	tagsMap := make(map[string]any)
	appender := NewTagAppender(t.allTagsAsFields, t.tagKeysAsFields, t.tagDotReplacement, tagsMap, tags)
	appender.AppendTagsForSpan(span, scope)
	return tags, tagsMap
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
				Key:   eventNameAttr,
				Type:  dbmodel.StringType,
				Value: event.Name(),
			})
		}
		event.Attributes().Range(func(k string, v pcommon.Value) bool {
			fields = append(fields, dbmodel.KeyValue{
				Key:   k,
				Type:  dbmodel.ValueType(v.Type().String()),
				Value: v.AsString(),
			})
			return true
		})
		logs = append(logs, dbmodel.Log{
			Timestamp: model.TimeAsEpochMicroseconds(event.Timestamp().AsTime()),
			Fields:    fields,
		})
	}
	return logs
}

func makeDBReferences(links ptrace.SpanLinkSlice, parentSpanID pcommon.SpanID, traceID pcommon.TraceID) []dbmodel.Reference {
	dbParentSpanId := dbmodel.SpanID(parentSpanID.String())
	dbTraceId := dbmodel.TraceID(traceID.String())
	refsCount := links.Len()
	if !parentSpanID.IsEmpty() {
		refsCount++
	}

	if refsCount == 0 {
		return nil
	}

	refs := make([]dbmodel.Reference, 0, refsCount)

	// Put parent span ID at the first place because usually backends look for it
	// as the first CHILD_OF item in the model.SpanRef slice.
	if !parentSpanID.IsEmpty() {
		refs = append(refs, dbmodel.Reference{
			TraceID: dbTraceId,
			SpanID:  dbParentSpanId,
			RefType: dbmodel.ChildOf,
		})
	}

	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		linkTraceID := link.TraceID()
		linkSpanID := link.SpanID()
		linkRefType := refTypeFromLink(link)
		if !parentSpanID.IsEmpty() && linkTraceID == traceID && linkSpanID == parentSpanID {
			// We already added a reference to this span, but maybe with the wrong type, so override.
			refs[0].RefType = linkRefType
			continue
		}
		refs = append(refs, dbmodel.Reference{
			TraceID: dbmodel.TraceID(linkTraceID.String()),
			SpanID:  dbmodel.SpanID(linkSpanID.String()),
			RefType: refTypeFromLink(link),
		})
	}

	return refs
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
	// that's not a dbmodel.ChildOf is a dbmodel.FollowsFrom
	return dbmodel.FollowsFrom
}

func (t ToDBModel) resourceToDbProcess(resource pcommon.Resource) *dbmodel.Process {
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
	tags, tagsMap := t.appendTagsFromResourceAttributes(attrs)
	process.Tags = tags
	process.Tag = tagsMap
	return process
}

func (t ToDBModel) appendTagsFromResourceAttributes(attrs pcommon.Map) ([]dbmodel.KeyValue, map[string]any) {
	tagsMap := make(map[string]any)
	kvs := make([]dbmodel.KeyValue, 0)
	if attrs.Len() == 0 {
		return kvs, tagsMap
	}
	appender := NewTagAppender(t.allTagsAsFields, t.tagKeysAsFields, t.tagDotReplacement, tagsMap, kvs)
	appender.AppendTagsFromResourceAttributes(attrs)
	if kvs == nil {
		kvs = make([]dbmodel.KeyValue, 0)
	}
	return kvs, tagsMap
}
