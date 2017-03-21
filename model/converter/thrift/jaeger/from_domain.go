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

func FromDomain(spans []*model.Span) []*jaeger.Span {
	jSpans := make([]*jaeger.Span, len(spans))
	dToJ := domainToJaegerTransformer{}
	for idx, span := range spans {
		jSpans[idx] = dToJ.transformSpan(span)
	}
	return jSpans
}

func FromDomainSpan(span *model.Span) *jaeger.Span {
	dToJ := &domainToJaegerTransformer{}
	return dToJ.transformSpan(span)
}

type domainToJaegerTransformer struct{}

func (d domainToJaegerTransformer) keyValueToTag(kv *model.KeyValue) *jaeger.Tag {
	if kv.VType == model.StringType {
		stringValue := string(kv.VStr)
		return &jaeger.Tag{
			Key:   kv.Key,
			VType: jaeger.TagType_STRING,
			VStr:  &stringValue,
		}
	}

	if kv.VType == model.Int64Type {
		intValue := kv.Int64()
		return &jaeger.Tag{
			Key:   kv.Key,
			VType: jaeger.TagType_LONG,
			VLong: &intValue,
		}
	}

	if kv.VType == model.BinaryType {
		binaryValue := kv.Binary()
		return &jaeger.Tag{
			Key:     kv.Key,
			VType:   jaeger.TagType_BINARY,
			VBinary: binaryValue,
		}
	}

	if kv.VType == model.BoolType {
		boolValue := false
		if kv.VNum > 0 {
			boolValue = true
		}
		return &jaeger.Tag{
			Key:   kv.Key,
			VType: jaeger.TagType_BOOL,
			VBool: &boolValue,
		}
	}

	if kv.VType == model.Float64Type {
		floatValue := kv.Float64()
		return &jaeger.Tag{
			Key:     kv.Key,
			VType:   jaeger.TagType_DOUBLE,
			VDouble: &floatValue,
		}
	}

	errString := fmt.Sprintf("No suitable tag type found for: %#v", kv.VType)
	errTag := &jaeger.Tag{
		Key:   "Error",
		VType: jaeger.TagType_STRING,
		VStr:  &errString,
	}

	return errTag
}

func (d domainToJaegerTransformer) convertKeyValuesToTags(kvs model.KeyValues) []*jaeger.Tag {
	jaegerTags := make([]*jaeger.Tag, len(kvs))
	for idx, kv := range kvs {
		jaegerTags[idx] = d.keyValueToTag(&kv)
	}
	return jaegerTags
}

func (d domainToJaegerTransformer) convertLogs(logs []model.Log) []*jaeger.Log {
	jaegerLogs := make([]*jaeger.Log, len(logs))
	for idx, log := range logs {
		jaegerLogs[idx] = &jaeger.Log{
			Timestamp: int64(model.TimeAsEpochMicroseconds(log.Timestamp)),
			Fields:    d.convertKeyValuesToTags(log.Fields),
		}
	}
	return jaegerLogs
}

func (d domainToJaegerTransformer) convertSpanRefs(refs []model.SpanRef) []*jaeger.SpanRef {
	jaegerSpanRefs := make([]*jaeger.SpanRef, len(refs))
	for idx, ref := range refs {
		jaegerSpanRefs[idx] = &jaeger.SpanRef{
			RefType:     jaeger.SpanRefType(ref.RefType),
			TraceIdLow:  int64(ref.TraceID.Low),
			TraceIdHigh: int64(ref.TraceID.High),
			SpanId:      int64(ref.SpanID),
		}
	}
	return jaegerSpanRefs
}

func (d domainToJaegerTransformer) transformSpan(span *model.Span) *jaeger.Span {
	tags := d.convertKeyValuesToTags(span.Tags)
	logs := d.convertLogs(span.Logs)
	refs := d.convertSpanRefs(span.References)

	jaegerSpan := &jaeger.Span{
		TraceIdLow:    int64(span.TraceID.Low),
		TraceIdHigh:   int64(span.TraceID.High),
		SpanId:        int64(span.SpanID),
		ParentSpanId:  int64(span.ParentSpanID),
		OperationName: span.OperationName,
		References:    refs,
		Flags:         int32(span.Flags),
		StartTime:     int64(model.TimeAsEpochMicroseconds(span.StartTime)),
		Duration:      int64(model.DurationAsMicroseconds(span.Duration)),
		Tags:          tags,
		Logs:          logs,
	}
	return jaegerSpan
}
