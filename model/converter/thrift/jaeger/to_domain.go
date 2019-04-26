// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jaeger

import (
	"fmt"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
)

// ToDomain transforms a set of spans and a process in jaeger.thrift format into a slice of model.Span.
// A valid []*model.Span is always returned, even when there are errors.
// Errors are presented as tags on spans
func ToDomain(jSpans []*jaeger.Span, jProcess *jaeger.Process) []*model.Span {
	return toDomain{}.ToDomain(jSpans, jProcess)
}

// ToDomainSpan transforms a span in jaeger.thrift format into model.Span.
// A valid model.Span is always returned, even when there are errors.
// Errors are presented as tags on spans
func ToDomainSpan(jSpan *jaeger.Span, jProcess *jaeger.Process) *model.Span {
	return toDomain{}.ToDomainSpan(jSpan, jProcess)
}

// ToDomainProcess transforms a process in jaeger.thrift format to model.Span.
func ToDomainProcess(jProcess *jaeger.Process) *model.Process {
	return toDomain{}.getProcess(jProcess)
}

type toDomain struct{}

func (td toDomain) ToDomain(jSpans []*jaeger.Span, jProcess *jaeger.Process) []*model.Span {
	spans := make([]*model.Span, len(jSpans))
	mProcess := td.getProcess(jProcess)
	for i, jSpan := range jSpans {
		spans[i] = td.transformSpan(jSpan, mProcess)
	}
	return spans
}

func (td toDomain) ToDomainSpan(jSpan *jaeger.Span, jProcess *jaeger.Process) *model.Span {
	mProcess := td.getProcess(jProcess)
	return td.transformSpan(jSpan, mProcess)
}

func (td toDomain) transformSpan(jSpan *jaeger.Span, mProcess *model.Process) *model.Span {
	traceID := model.NewTraceID(uint64(jSpan.TraceIdHigh), uint64(jSpan.TraceIdLow))
	//allocate extra space for future append operation
	tags := td.getTags(jSpan.Tags, 1)
	refs := td.getReferences(jSpan.References)
	// We no longer store ParentSpanID in the domain model, but the data in Thrift model
	// might still have these IDs without representing them in the References, so we
	// convert it back into child-of reference.
	if jSpan.ParentSpanId != 0 {
		parentSpanID := model.NewSpanID(uint64(jSpan.ParentSpanId))
		refs = model.MaybeAddParentSpanID(traceID, parentSpanID, refs)
	}
	return &model.Span{
		TraceID:       traceID,
		SpanID:        model.NewSpanID(uint64(jSpan.SpanId)),
		OperationName: jSpan.OperationName,
		References:    refs,
		Flags:         model.Flags(jSpan.Flags),
		StartTime:     model.EpochMicrosecondsAsTime(uint64(jSpan.StartTime)),
		Duration:      model.MicrosecondsAsDuration(uint64(jSpan.Duration)),
		Tags:          tags,
		Logs:          td.getLogs(jSpan.Logs),
		Process:       mProcess,
	}
}

func (td toDomain) getReferences(jRefs []*jaeger.SpanRef) []model.SpanRef {
	if len(jRefs) == 0 {
		return nil
	}

	mRefs := make([]model.SpanRef, len(jRefs))
	for idx, jRef := range jRefs {
		mRefs[idx] = model.SpanRef{
			RefType: model.SpanRefType(int(jRef.RefType)),
			TraceID: model.NewTraceID(uint64(jRef.TraceIdHigh), uint64(jRef.TraceIdLow)),
			SpanID:  model.NewSpanID(uint64(jRef.SpanId)),
		}
	}

	return mRefs
}

// getProcess takes a jaeger.thrift process and produces a model.Process.
// Any errors are presented as tags
func (td toDomain) getProcess(jProcess *jaeger.Process) *model.Process {
	if jProcess == nil {
		return nil
	}
	tags := td.getTags(jProcess.Tags, 0)
	return &model.Process{
		Tags:        tags,
		ServiceName: jProcess.ServiceName,
	}
}

//convert the jaeger.Tag slice to domain KeyValue slice
//zipkin/to_domain.go does not give a default slice size since it has to filter annotations, jaeger conversion is more predictable
//thus to avoid future full array copy when using append, pre-allocate extra space as an optimization
func (td toDomain) getTags(tags []*jaeger.Tag, extraSpace int) model.KeyValues {
	if len(tags) == 0 {
		return nil
	}
	retMe := make(model.KeyValues, len(tags), len(tags)+extraSpace)
	for i, tag := range tags {
		retMe[i] = td.getTag(tag)
	}
	return retMe
}

func (td toDomain) getTag(tag *jaeger.Tag) model.KeyValue {
	switch tag.VType {
	case jaeger.TagType_BOOL:
		return model.Bool(tag.Key, tag.GetVBool())
	case jaeger.TagType_BINARY:
		return model.Binary(tag.Key, tag.GetVBinary())
	case jaeger.TagType_DOUBLE:
		return model.Float64(tag.Key, tag.GetVDouble())
	case jaeger.TagType_LONG:
		return model.Int64(tag.Key, tag.GetVLong())
	case jaeger.TagType_STRING:
		return model.String(tag.Key, tag.GetVStr())
	default:
		return model.String(tag.Key, fmt.Sprintf("Unknown VType: %+v", tag))
	}
}

func (td toDomain) getLogs(logs []*jaeger.Log) []model.Log {
	if len(logs) == 0 {
		return nil
	}
	retMe := make([]model.Log, len(logs))
	for i, log := range logs {
		retMe[i] = model.Log{
			Timestamp: model.EpochMicrosecondsAsTime(uint64(log.Timestamp)),
			Fields:    td.getTags(log.Fields, 0),
		}
	}
	return retMe
}
