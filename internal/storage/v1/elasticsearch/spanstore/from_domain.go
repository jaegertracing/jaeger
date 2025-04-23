// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"strings"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

// NewFromDomain creates FromDomain used to convert model span to db span
func NewFromDomain() FromDomain {
	return FromDomain{}
}

// FromDomain is used to convert model span to db span
type FromDomain struct{}

// FromDomainEmbedProcess converts model.Span into json.Span format.
// This format includes a ParentSpanID and an embedded Process.
func (fd FromDomain) FromDomainEmbedProcess(span *model.Span) *dbmodel.Span {
	s := fd.convertSpanInternal(span)
	s.Process = fd.convertProcess(span.Process)
	s.References = fd.convertReferences(span)
	return &s
}

func (fd FromDomain) convertSpanInternal(span *model.Span) dbmodel.Span {
	tags := fd.convertKeyValues(span.Tags)
	return dbmodel.Span{
		TraceID:         dbmodel.TraceID(span.TraceID.String()),
		SpanID:          dbmodel.SpanID(span.SpanID.String()),
		Flags:           uint32(span.Flags),
		OperationName:   span.OperationName,
		StartTime:       model.TimeAsEpochMicroseconds(span.StartTime),
		StartTimeMillis: model.TimeAsEpochMicroseconds(span.StartTime) / 1000,
		Duration:        model.DurationAsMicroseconds(span.Duration),
		Tags:            tags,
		Logs:            fd.convertLogs(span.Logs),
	}
}

func (fd FromDomain) convertReferences(span *model.Span) []dbmodel.Reference {
	out := make([]dbmodel.Reference, 0, len(span.References))
	for _, ref := range span.References {
		out = append(out, dbmodel.Reference{
			RefType: fd.convertRefType(ref.RefType),
			TraceID: dbmodel.TraceID(ref.TraceID.String()),
			SpanID:  dbmodel.SpanID(ref.SpanID.String()),
		})
	}
	return out
}

func (FromDomain) convertRefType(refType model.SpanRefType) dbmodel.ReferenceType {
	if refType == model.FollowsFrom {
		return dbmodel.FollowsFrom
	}
	return dbmodel.ChildOf
}

func (fd FromDomain) convertKeyValues(keyValues model.KeyValues) []dbmodel.KeyValue {
	kvs := make([]dbmodel.KeyValue, 0, len(keyValues))
	for i := range keyValues {
		kvs = append(kvs, fd.convertKeyValue(keyValues[i]))
	}
	if kvs == nil {
		kvs = make([]dbmodel.KeyValue, 0)
	}
	return kvs
}

func (fd FromDomain) convertLogs(logs []model.Log) []dbmodel.Log {
	out := make([]dbmodel.Log, len(logs))
	for i, log := range logs {
		var kvs []dbmodel.KeyValue
		for _, kv := range log.Fields {
			kvs = append(kvs, fd.convertKeyValue(kv))
		}
		out[i] = dbmodel.Log{
			Timestamp: model.TimeAsEpochMicroseconds(log.Timestamp),
			Fields:    kvs,
		}
	}
	return out
}

func (fd FromDomain) convertProcess(process *model.Process) dbmodel.Process {
	tags := fd.convertKeyValues(process.Tags)
	return dbmodel.Process{
		ServiceName: process.ServiceName,
		Tags:        tags,
	}
}

func (FromDomain) convertKeyValue(kv model.KeyValue) dbmodel.KeyValue {
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
