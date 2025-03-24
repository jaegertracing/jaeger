// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package jaeger

import (
	"fmt"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
)

// FromDomain takes an array of model.Span and returns
// an array of jaeger.Span.  If errors are found during
// conversion of tags, then error tags are appended.
func FromDomain(spans []*model.Span) []*jaeger.Span {
	jSpans := make([]*jaeger.Span, len(spans))
	dToJ := domainToJaegerTransformer{}
	for idx, span := range spans {
		jSpans[idx] = dToJ.transformSpan(span)
	}
	return jSpans
}

// FromDomainSpan takes a single model.Span and
// converts it into a jaeger.Span.  If errors are found
// during conversion of tags, then error tags are appended.
func FromDomainSpan(span *model.Span) *jaeger.Span {
	dToJ := &domainToJaegerTransformer{}
	return dToJ.transformSpan(span)
}

type domainToJaegerTransformer struct{}

func (domainToJaegerTransformer) keyValueToTag(kv *model.KeyValue) *jaeger.Tag {
	if kv.VType == model.StringType {
		stringValue := kv.VStr
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
		boolValue := kv.Bool()
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
			//nolint: gosec // G115
			Timestamp: int64(model.TimeAsEpochMicroseconds(log.Timestamp)),
			Fields:    d.convertKeyValuesToTags(log.Fields),
		}
	}
	return jaegerLogs
}

func (domainToJaegerTransformer) convertSpanRefs(refs []model.SpanRef) []*jaeger.SpanRef {
	jaegerSpanRefs := make([]*jaeger.SpanRef, len(refs))
	for idx, ref := range refs {
		//nolint: gosec // G115
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

	//nolint: gosec // G115
	jaegerSpan := &jaeger.Span{
		TraceIdLow:    int64(span.TraceID.Low),
		TraceIdHigh:   int64(span.TraceID.High),
		SpanId:        int64(span.SpanID),
		ParentSpanId:  int64(span.ParentSpanID()),
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
