// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"fmt"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	// warningStringPrefix is a magic string prefix for tag names to store span warnings.
	warningStringPrefix = "$$span.warning."
)

var (
	domainToDBRefMap = map[model.SpanRefType]string{
		model.SpanRefType_CHILD_OF:     ChildOf,
		model.SpanRefType_FOLLOWS_FROM: FollowsFrom,
	}

	domainToDBValueTypeMap = map[model.ValueType]string{
		model.StringType:  StringType,
		model.BoolType:    BoolType,
		model.Int64Type:   Int64Type,
		model.Float64Type: Float64Type,
		model.BinaryType:  BinaryType,
	}
)

// FromDomain converts a domain model.Span to a database Span
func FromDomain(span *model.Span) *Span {
	return converter{}.fromDomain(span)
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
	spanHash, _ := model.HashCode(span)

	tags = append(tags, warnings...)

	//nolint:gosec // G115
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
			//nolint:gosec // G115
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
			//nolint:gosec // G115
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
