// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"fmt"
	"strings"

	"github.com/jaegertracing/jaeger/model"
)

const (
	// warningStringPrefix is a magic string prefix for tag names to store span warnings.
	warningStringPrefix = "$$span.warning."
)

var (
	dbToDomainRefMap = map[string]model.SpanRefType{
		childOf:     model.SpanRefType_CHILD_OF,
		followsFrom: model.SpanRefType_FOLLOWS_FROM,
	}

	domainToDBRefMap = map[model.SpanRefType]string{
		model.SpanRefType_CHILD_OF:     childOf,
		model.SpanRefType_FOLLOWS_FROM: followsFrom,
	}

	domainToDBValueTypeMap = map[model.ValueType]string{
		model.StringType:  stringType,
		model.BoolType:    boolType,
		model.Int64Type:   int64Type,
		model.Float64Type: float64Type,
		model.BinaryType:  binaryType,
	}
)

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
	warnings := c.toDBWarnings(span.Warnings)
	logs := c.toDBLogs(span.Logs)
	refs := c.toDBRefs(span.References)
	udtProcess := c.toDBProcess(span.Process)
	spanHash, found := span.GetHashTag()
	if !found {
		tempSpam := *span
		spanHash, _ = tempSpam.SetHashTag()
	}

	tags = append(tags, warnings...)

	//nolint: gosec // G115
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
		SpanHash:      spanHash,
	}
}

func (c converter) toDomain(dbSpan *Span) (*model.Span, error) {
	tags, err := c.fromDBTags(dbSpan.Tags)
	if err != nil {
		return nil, err
	}
	warnings, err := c.fromDBWarnings(dbSpan.Tags)
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
		TraceID: traceID,
		//nolint: gosec // G115
		SpanID:        model.NewSpanID(uint64(dbSpan.SpanID)),
		OperationName: dbSpan.OperationName,
		//nolint: gosec // G115
		References: model.MaybeAddParentSpanID(traceID, model.NewSpanID(uint64(dbSpan.ParentID)), refs),
		//nolint: gosec // G115
		Flags: model.Flags(uint32(dbSpan.Flags)),
		//nolint: gosec // G115
		StartTime: model.EpochMicrosecondsAsTime(uint64(dbSpan.StartTime)),
		//nolint: gosec // G115
		Duration: model.MicrosecondsAsDuration(uint64(dbSpan.Duration)),
		Tags:     tags,
		Warnings: warnings,
		Logs:     logs,
		Process:  process,
	}
	return span, nil
}

func (c converter) fromDBTags(tags []KeyValue) ([]model.KeyValue, error) {
	retMe := make([]model.KeyValue, 0, len(tags))
	for i := range tags {
		if strings.HasPrefix(tags[i].Key, warningStringPrefix) {
			continue
		}
		kv, err := c.fromDBTag(&tags[i])
		if err != nil {
			return nil, err
		}
		retMe = append(retMe, kv)
	}
	return retMe, nil
}

func (c converter) fromDBWarnings(tags []KeyValue) ([]string, error) {
	var retMe []string
	for _, tag := range tags {
		if !strings.HasPrefix(tag.Key, warningStringPrefix) {
			continue
		}
		kv, err := c.fromDBTag(&tag)
		if err != nil {
			return nil, err
		}
		retMe = append(retMe, kv.VStr)
	}
	return retMe, nil
}

func (converter) fromDBTag(tag *KeyValue) (model.KeyValue, error) {
	switch tag.ValueType {
	case stringType:
		return model.String(tag.Key, tag.ValueString), nil
	case boolType:
		return model.Bool(tag.Key, tag.ValueBool), nil
	case int64Type:
		return model.Int64(tag.Key, tag.ValueInt64), nil
	case float64Type:
		return model.Float64(tag.Key, tag.ValueFloat64), nil
	case binaryType:
		return model.Binary(tag.Key, tag.ValueBinary), nil
	}
	return model.KeyValue{}, fmt.Errorf("invalid ValueType in %+v", tag)
}

func (c converter) fromDBLogs(logs []Log) ([]model.Log, error) {
	retMe := make([]model.Log, len(logs))
	for i, l := range logs {
		fields, err := c.fromDBTags(l.Fields)
		if err != nil {
			return nil, err
		}
		retMe[i] = model.Log{
			//nolint: gosec // G115
			Timestamp: model.EpochMicrosecondsAsTime(uint64(l.Timestamp)),
			Fields:    fields,
		}
	}
	return retMe, nil
}

func (converter) fromDBRefs(refs []SpanRef) ([]model.SpanRef, error) {
	retMe := make([]model.SpanRef, len(refs))
	for i, r := range refs {
		refType, ok := dbToDomainRefMap[r.RefType]
		if !ok {
			return nil, fmt.Errorf("invalid SpanRefType in %+v", r)
		}
		retMe[i] = model.SpanRef{
			RefType: refType,
			TraceID: r.TraceID.ToDomain(),
			//nolint: gosec // G115
			SpanID: model.NewSpanID(uint64(r.SpanID)),
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

func (converter) toDBTags(tags []model.KeyValue) []KeyValue {
	retMe := make([]KeyValue, len(tags))
	for i, t := range tags {
		// do we want to validate a jaeger tag here? Making sure that the type and value matches up?
		retMe[i] = KeyValue{
			Key:          t.Key,
			ValueType:    domainToDBValueTypeMap[t.VType],
			ValueString:  t.VStr,
			ValueBool:    t.Bool(),
			ValueInt64:   t.Int64(),
			ValueFloat64: t.Float64(),
			ValueBinary:  t.Binary(),
		}
	}
	return retMe
}

func (converter) toDBWarnings(warnings []string) []KeyValue {
	retMe := make([]KeyValue, len(warnings))
	for i, w := range warnings {
		kv := model.String(fmt.Sprintf("%s%d", warningStringPrefix, i+1), w)
		retMe[i] = KeyValue{
			Key:         kv.Key,
			ValueType:   domainToDBValueTypeMap[kv.VType],
			ValueString: kv.VStr,
		}
	}
	return retMe
}

func (c converter) toDBLogs(logs []model.Log) []Log {
	retMe := make([]Log, len(logs))
	for i, l := range logs {
		retMe[i] = Log{
			//nolint: gosec // G115
			Timestamp: int64(model.TimeAsEpochMicroseconds(l.Timestamp)),
			Fields:    c.toDBTags(l.Fields),
		}
	}
	return retMe
}

func (converter) toDBRefs(refs []model.SpanRef) []SpanRef {
	retMe := make([]SpanRef, len(refs))
	for i, r := range refs {
		retMe[i] = SpanRef{
			TraceID: TraceIDFromDomain(r.TraceID),
			//nolint: gosec // G115
			SpanID:  int64(r.SpanID),
			RefType: domainToDBRefMap[r.RefType],
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
