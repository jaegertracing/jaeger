// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"strings"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

// FromDomainEmbedProcess converts model.Span into json.Span format.
// This format includes a ParentSpanID and an embedded Process.
func FromDomainEmbedProcess(span *model.Span) *dbmodel.Span {
	s := convertSpanInternal(span)
	s.Process = convertProcess(span.Process)
	s.References = convertReferences(span)
	return &s
}

func convertSpanInternal(span *model.Span) dbmodel.Span {
	tags := convertKeyValues(span.Tags)
	return dbmodel.Span{
		TraceID:         dbmodel.TraceID(span.TraceID.String()),
		SpanID:          dbmodel.SpanID(span.SpanID.String()),
		Flags:           uint32(span.Flags),
		OperationName:   span.OperationName,
		StartTime:       model.TimeAsEpochMicroseconds(span.StartTime),
		StartTimeMillis: model.TimeAsEpochMicroseconds(span.StartTime) / 1000,
		Duration:        model.DurationAsMicroseconds(span.Duration),
		Tags:            tags,
		Logs:            convertLogs(span.Logs),
	}
}

func convertReferences(span *model.Span) []dbmodel.Reference {
	out := make([]dbmodel.Reference, 0, len(span.References))
	for _, ref := range span.References {
		out = append(out, dbmodel.Reference{
			RefType: convertRefType(ref.RefType),
			TraceID: dbmodel.TraceID(ref.TraceID.String()),
			SpanID:  dbmodel.SpanID(ref.SpanID.String()),
		})
	}
	return out
}

func convertRefType(refType model.SpanRefType) dbmodel.ReferenceType {
	if refType == model.FollowsFrom {
		return dbmodel.FollowsFrom
	}
	return dbmodel.ChildOf
}

func convertKeyValues(keyValues model.KeyValues) []dbmodel.KeyValue {
	var kvs []dbmodel.KeyValue
	for _, kv := range keyValues {
		kvs = append(kvs, convertKeyValue(kv))
	}
	if kvs == nil {
		kvs = make([]dbmodel.KeyValue, 0)
	}
	return kvs
}

func convertLogs(logs []model.Log) []dbmodel.Log {
	out := make([]dbmodel.Log, len(logs))
	for i, log := range logs {
		var kvs []dbmodel.KeyValue
		for _, kv := range log.Fields {
			kvs = append(kvs, convertKeyValue(kv))
		}
		out[i] = dbmodel.Log{
			Timestamp: model.TimeAsEpochMicroseconds(log.Timestamp),
			Fields:    kvs,
		}
	}
	return out
}

func convertProcess(process *model.Process) dbmodel.Process {
	tags := convertKeyValues(process.Tags)
	return dbmodel.Process{
		ServiceName: process.ServiceName,
		Tags:        tags,
	}
}

func convertKeyValue(kv model.KeyValue) dbmodel.KeyValue {
	outKv := dbmodel.KeyValue{
		Key:  kv.Key,
		Type: dbmodel.ValueType(strings.ToLower(kv.VType.String())),
	}
	if kv.GetVType() == model.BinaryType {
		outKv.Value = kv.AsString()
	} else {
		outKv.Value = kv.Value()
	}
	return outKv
}
