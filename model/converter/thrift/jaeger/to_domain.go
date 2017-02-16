// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package jaeger

import (
	"fmt"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/thrift-gen/jaeger"
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

// toDomain is a private struct that namespaces some conversion functions. It has access to its own private utility functions
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
	tags := td.getTags(jSpan.Tags)
	return &model.Span{
		TraceID: model.TraceID{
			High: uint64(jSpan.TraceIdHigh),
			Low:  uint64(jSpan.TraceIdLow),
		},
		SpanID:        model.SpanID(jSpan.SpanId),
		OperationName: jSpan.OperationName,
		ParentSpanID:  model.SpanID(jSpan.GetParentSpanId()),
		Flags:         model.Flags(jSpan.Flags),
		StartTime:     model.EpochMicrosecondsAsTime(uint64(jSpan.StartTime)),
		Duration:      model.MicrosecondsAsDuration(uint64(jSpan.Duration)),
		Tags:          tags,
		Logs:          td.getLogs(jSpan.Logs),
		Process:       mProcess,
	}
}

// getProcess takes a jaeger.thrift process and produces a model.Process.
// Any errors are presented as tags
func (td toDomain) getProcess(jProcess *jaeger.Process) *model.Process {
	tags := td.getTags(jProcess.Tags)
	return &model.Process{
		Tags:        tags,
		ServiceName: jProcess.ServiceName,
	}
}

func (td toDomain) getTags(tags []*jaeger.Tag) model.KeyValues {
	if len(tags) == 0 {
		return nil
	}
	retMe := make(model.KeyValues, len(tags))
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
			Fields:    td.getTags(log.Fields),
		}
	}
	return retMe
}
