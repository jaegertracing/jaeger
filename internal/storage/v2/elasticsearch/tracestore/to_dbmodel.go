// Copyright (c) 2025 The Jaeger Authors.
// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Code originally copied from https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/e49500a9b68447cbbe237fa29526ba99e4963f39/pkg/translator/jaeger/traces_to_jaegerproto.go

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
	tagW3CTraceState = "w3c.tracestate"
	tagHTTPStatusMsg = "http.status_message"
	tagError         = "error"
)

// NewToDBModel creates FromDomain used to convert model span to db span
func NewToDBModel(allTagsAsObject bool, tagKeysAsFields []string, tagDotReplacement string) ToDBModel {
	tags := map[string]bool{}
	for _, k := range tagKeysAsFields {
		tags[k] = true
	}
	return ToDBModel{allTagsAsFields: allTagsAsObject, tagKeysAsFields: tags, tagDotReplacement: tagDotReplacement}
}

// ToDBModel is used to convert model span to db span
type ToDBModel struct {
	allTagsAsFields   bool
	tagKeysAsFields   map[string]bool
	tagDotReplacement string
}

// ConvertToDBModel translates internal trace data into the DB Spans.
// Returns slice of translated DB Spans and error if translation failed.
func (t ToDBModel) ConvertToDBModel(td ptrace.Traces) []dbmodel.Span {
	resourceSpans := td.ResourceSpans()

	if resourceSpans.Len() == 0 {
		return nil
	}

	batches := make([]dbmodel.Span, 0, resourceSpans.Len())
	for i := 0; i < resourceSpans.Len(); i++ {
		rs := resourceSpans.At(i)
		batch := t.resourceSpansToDbSpans(rs)
		if batch != nil {
			batches = append(batches, batch...)
		}
	}

	return batches
}

func (t ToDBModel) resourceSpansToDbSpans(resourceSpans ptrace.ResourceSpans) []dbmodel.Span {
	resource := resourceSpans.Resource()
	scopeSpans := resourceSpans.ScopeSpans()

	if scopeSpans.Len() == 0 {
		return []dbmodel.Span{}
	}

	process := t.resourceToDbProcess(resource)

	// Approximate the number of the spans as the number of the spans in the first
	// instrumentation library info.
	dbSpans := make([]dbmodel.Span, 0, scopeSpans.At(0).Spans().Len())

	for _, scopeSpan := range scopeSpans.All() {
		for _, span := range scopeSpan.Spans().All() {
			dbSpan := t.spanToDbSpan(span, scopeSpan.Scope(), process)
			dbSpans = append(dbSpans, dbSpan)
		}
	}

	return dbSpans
}

func (t ToDBModel) resourceToDbProcess(resource pcommon.Resource) dbmodel.Process {
	process := dbmodel.Process{}
	attrs := resource.Attributes()
	if attrs.Len() == 0 {
		process.ServiceName = noServiceName
		return process
	}
	appender := newTagAppender(t.allTagsAsFields, t.tagKeysAsFields, t.tagDotReplacement)
	for key, attr := range attrs.All() {
		if key == conventions.AttributeServiceName {
			process.ServiceName = attr.AsString()
			continue
		}
		appender.appendTag(key, attr)
	}
	tags, tagMap := appender.getTags()
	process.Tags = tags
	process.Tag = tagMap
	return process
}

func appendTagsFromAttributes(dest []dbmodel.KeyValue, attrs pcommon.Map) []dbmodel.KeyValue {
	for key, attr := range attrs.All() {
		dest = append(dest, attributeToDbTag(key, attr))
	}
	return dest
}

func (t ToDBModel) spanToDbSpan(span ptrace.Span, libraryTags pcommon.InstrumentationScope, process dbmodel.Process) dbmodel.Span {
	traceID := dbmodel.TraceID(span.TraceID().String())
	parentSpanID := dbmodel.SpanID(span.ParentSpanID().String())
	startTime := span.StartTimestamp().AsTime()
	tags, tagMap := t.getDbSpanTags(span, libraryTags)
	return dbmodel.Span{
		TraceID:         traceID,
		SpanID:          dbmodel.SpanID(span.SpanID().String()),
		OperationName:   span.Name(),
		References:      linksToDbSpanRefs(span.Links(), parentSpanID, traceID),
		StartTime:       model.TimeAsEpochMicroseconds(startTime),
		StartTimeMillis: model.TimeAsEpochMicroseconds(startTime) / 1000,
		Duration:        model.DurationAsMicroseconds(span.EndTimestamp().AsTime().Sub(startTime)),
		Tags:            tags,
		Tag:             tagMap,
		Logs:            spanEventsToDbSpanLogs(span.Events()),
		Process:         process,
		Flags:           span.Flags(),
	}
}

func (t ToDBModel) getDbSpanTags(span ptrace.Span, scope pcommon.InstrumentationScope) ([]dbmodel.KeyValue, map[string]any) {
	appender := newTagAppender(t.allTagsAsFields, t.tagKeysAsFields, t.tagDotReplacement)
	appender.appendInstrumentationLibraryTags(scope)
	appender.appendSpanKindTag(span.Kind())
	status := span.Status()
	appender.appendStatusCodeTag(status.Code())
	appender.appendStatusMsgTag(status.Message())
	appender.appendTraceStateTag(span.TraceState().AsRaw())
	appender.appendTags(span.Attributes())
	return appender.getTags()
}

// linksToDbSpanRefs constructs jaeger span references based on parent span ID and span links.
// The parent span ID is used to add a CHILD_OF reference, _unless_ it is referenced from one of the links.
func linksToDbSpanRefs(links ptrace.SpanLinkSlice, parentSpanID dbmodel.SpanID, traceID dbmodel.TraceID) []dbmodel.Reference {
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
