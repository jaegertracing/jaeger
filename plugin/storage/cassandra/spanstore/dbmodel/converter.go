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

package dbmodel

import (
	"errors"

	"github.com/jaegertracing/jaeger/model"
)

// ErrUnknownKeyValueTypeFromCassandra is an error that occurs when trying to decipher an unknown tag type from the database
var ErrUnknownKeyValueTypeFromCassandra = errors.New("Unknown tag type found in Cassandra")

// FromDomain converts a domain model.Span to a database Span
func FromDomain(span *model.Span) *Span {
	return converter{}.fromDomain(span)
}

// ToDomain converts a database Span to a domain model.Span
func ToDomain(dbSpan *Span) (*model.Span, error) {
	return converter{}.toDomain(dbSpan)
}

// converter converts Spans between domain and database representations.
// It primarily exists to namespace the conversion functions.
type converter struct{}

func (c converter) fromDomain(span *model.Span) *Span {
	tags := c.toDBTags(span.Tags)
	logs := c.toDBLogs(span.Logs)
	refs := c.toDBRefs(span.References)
	udtProcess := c.toDBProcess(span.Process)
	spanHash, _ := model.HashCode(span)

	return &Span{
		TraceID:       TraceIDFromDomain(span.TraceID),
		SpanID:        int64(span.SpanID),
		OperationName: span.OperationName,
		Flags:         int32(span.Flags),
		StartTime:     int64(model.TimeAsEpochMicroseconds(span.StartTime)),
		Duration:      int64(model.DurationAsMicroseconds(span.Duration)),
		Tags:          tags,
		Logs:          logs,
		Refs:          refs,
		Process:       udtProcess,
		ServiceName:   span.Process.ServiceName,
		SpanHash:      int64(spanHash),
	}
}

func (c converter) toDomain(dbSpan *Span) (*model.Span, error) {
	tags, err := c.fromDBTags(dbSpan.Tags)
	if err != nil {
		return nil, err
	}
	logs, err := c.fromDBLogs(dbSpan.Logs)
	if err != nil {
		return nil, err
	}
	refs, err := c.fromDBRefs(dbSpan.Refs)
	if err != nil {
		return nil, err
	}
	process, err := c.fromDBProcess(dbSpan.Process)
	if err != nil {
		return nil, err
	}
	traceID := dbSpan.TraceID.ToDomain()
	span := &model.Span{
		TraceID:       traceID,
		SpanID:        model.NewSpanID(uint64(dbSpan.SpanID)),
		OperationName: dbSpan.OperationName,
		References:    model.MaybeAddParentSpanID(traceID, model.NewSpanID(uint64(dbSpan.ParentID)), refs),
		Flags:         model.Flags(uint32(dbSpan.Flags)),
		StartTime:     model.EpochMicrosecondsAsTime(uint64(dbSpan.StartTime)),
		Duration:      model.MicrosecondsAsDuration(uint64(dbSpan.Duration)),
		Tags:          tags,
		Logs:          logs,
		Process:       process,
	}
	return span, nil
}

func (c converter) fromDBTags(tags []KeyValue) ([]model.KeyValue, error) {
	retMe := make([]model.KeyValue, len(tags))
	for i := range tags {
		kv, err := c.fromDBTag(&tags[i])
		if err != nil {
			return nil, err
		}
		retMe[i] = kv
	}
	return retMe, nil
}

func (c converter) fromDBTag(tag *KeyValue) (model.KeyValue, error) {
	vType, err := model.ValueTypeFromString(tag.ValueType)
	if err != nil {
		return model.KeyValue{}, err
	}
	return c.fromDBTagOfType(tag, vType)
}

func (c converter) fromDBTagOfType(tag *KeyValue, vType model.ValueType) (model.KeyValue, error) {
	switch vType {
	case model.StringType:
		return model.String(tag.Key, tag.ValueString), nil
	case model.BoolType:
		return model.Bool(tag.Key, tag.ValueBool), nil
	case model.Int64Type:
		return model.Int64(tag.Key, tag.ValueInt64), nil
	case model.Float64Type:
		return model.Float64(tag.Key, tag.ValueFloat64), nil
	case model.BinaryType:
		return model.Binary(tag.Key, tag.ValueBinary), nil
	}
	return model.KeyValue{}, ErrUnknownKeyValueTypeFromCassandra
}

func (c converter) fromDBLogs(logs []Log) ([]model.Log, error) {
	retMe := make([]model.Log, len(logs))
	for i, l := range logs {
		fields, err := c.fromDBTags(l.Fields)
		if err != nil {
			return nil, err
		}
		retMe[i] = model.Log{
			Timestamp: model.EpochMicrosecondsAsTime(uint64(l.Timestamp)),
			Fields:    fields,
		}
	}
	return retMe, nil
}

func (c converter) fromDBRefs(refs []SpanRef) ([]model.SpanRef, error) {
	retMe := make([]model.SpanRef, len(refs))
	for i, r := range refs {
		refType, err := model.SpanRefTypeFromString(r.RefType)
		if err != nil {
			return nil, err
		}
		retMe[i] = model.SpanRef{
			RefType: refType,
			TraceID: r.TraceID.ToDomain(),
			SpanID:  model.NewSpanID(uint64(r.SpanID)),
		}
	}
	return retMe, nil
}

func (c converter) fromDBProcess(process Process) (*model.Process, error) {
	tags, err := c.fromDBTags(process.Tags)
	if err != nil {
		return nil, err
	}
	return &model.Process{
		Tags:        tags,
		ServiceName: process.ServiceName,
	}, nil
}

func (c converter) toDBTags(tags []model.KeyValue) []KeyValue {
	retMe := make([]KeyValue, len(tags))
	for i, t := range tags {
		// do we want to validate a jaeger tag here? Making sure that the type and value matches up?
		retMe[i] = KeyValue{
			Key:          t.Key,
			ValueType:    t.VType.String(),
			ValueString:  t.VStr,
			ValueBool:    t.Bool(),
			ValueInt64:   t.Int64(),
			ValueFloat64: t.Float64(),
			ValueBinary:  t.Binary(),
		}
	}
	return retMe
}

func (c converter) toDBLogs(logs []model.Log) []Log {
	retMe := make([]Log, len(logs))
	for i, l := range logs {
		retMe[i] = Log{
			Timestamp: int64(model.TimeAsEpochMicroseconds(l.Timestamp)),
			Fields:    c.toDBTags(l.Fields),
		}
	}
	return retMe
}

func (c converter) toDBRefs(refs []model.SpanRef) []SpanRef {
	retMe := make([]SpanRef, len(refs))
	for i, r := range refs {
		retMe[i] = SpanRef{
			TraceID: TraceIDFromDomain(r.TraceID),
			SpanID:  int64(r.SpanID),
			RefType: r.RefType.String(),
		}
	}
	return retMe
}

func (c converter) toDBProcess(process *model.Process) Process {
	return Process{
		ServiceName: process.ServiceName,
		Tags:        c.toDBTags(process.Tags),
	}
}
