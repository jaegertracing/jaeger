// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"fmt"
	"strings"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

const (
	jsMaxSafeInteger = int64(1)<<53 - 1
	jsMinSafeInteger = -jsMaxSafeInteger
)

// FromDomain converts model.Trace into json.Trace format.
// It assumes that the domain model is valid, namely that all enums
// have valid values, so that it does not need to check for errors.
func FromDomain(trace *model.Trace) *Trace {
	fd := fromDomain{}
	fd.convertKeyValuesFunc = fd.convertKeyValues
	return fd.fromDomain(trace)
}

// FromDomainEmbedProcess converts model.Span into json.Span format.
// This format includes a ParentSpanID and an embedded Process.
func FromDomainEmbedProcess(span *model.Span) *Span {
	fd := fromDomain{}
	fd.convertKeyValuesFunc = fd.convertKeyValuesString
	return fd.convertSpanEmbedProcess(span)
}

type fromDomain struct {
	convertKeyValuesFunc func(keyValues model.KeyValues) []KeyValue
}

func (fd fromDomain) fromDomain(trace *model.Trace) *Trace {
	jSpans := make([]Span, len(trace.Spans))
	processes := &processHashtable{}
	var traceID TraceID
	for i, span := range trace.Spans {
		if i == 0 {
			traceID = TraceID(span.TraceID.String())
		}
		processID := ProcessID(processes.getKey(span.Process))
		jSpans[i] = fd.convertSpan(span, processID)
	}
	jTrace := &Trace{
		TraceID:   traceID,
		Spans:     jSpans,
		Processes: fd.convertProcesses(processes.getMapping()),
		Warnings:  trace.Warnings,
	}
	return jTrace
}

func (fd fromDomain) convertSpanInternal(span *model.Span) Span {
	return Span{
		TraceID:       TraceID(span.TraceID.String()),
		SpanID:        SpanID(span.SpanID.String()),
		Flags:         uint32(span.Flags),
		OperationName: span.OperationName,
		StartTime:     model.TimeAsEpochMicroseconds(span.StartTime),
		Duration:      model.DurationAsMicroseconds(span.Duration),
		Tags:          fd.convertKeyValuesFunc(span.Tags),
		Logs:          fd.convertLogs(span.Logs),
	}
}

func (fd fromDomain) convertSpan(span *model.Span, processID ProcessID) Span {
	s := fd.convertSpanInternal(span)
	s.ProcessID = processID
	s.Warnings = span.Warnings
	s.References = fd.convertReferences(span)
	return s
}

func (fd fromDomain) convertSpanEmbedProcess(span *model.Span) *Span {
	s := fd.convertSpanInternal(span)
	process := fd.convertProcess(span.Process)
	s.Process = &process
	s.References = fd.convertReferences(span)
	return &s
}

func (fd fromDomain) convertReferences(span *model.Span) []Reference {
	out := make([]Reference, 0, len(span.References))
	for _, ref := range span.References {
		out = append(out, Reference{
			RefType: fd.convertRefType(ref.RefType),
			TraceID: TraceID(ref.TraceID.String()),
			SpanID:  SpanID(ref.SpanID.String()),
		})
	}
	return out
}

func (fromDomain) convertRefType(refType model.SpanRefType) ReferenceType {
	if refType == model.FollowsFrom {
		return FollowsFrom
	}
	return ChildOf
}

func (fromDomain) convertKeyValues(keyValues model.KeyValues) []KeyValue {
	out := make([]KeyValue, len(keyValues))
	for i, kv := range keyValues {
		var value any
		switch kv.VType {
		case model.StringType:
			value = kv.VStr
		case model.BoolType:
			value = kv.Bool()
		case model.Int64Type:
			value = kv.Int64()
			if kv.Int64() > jsMaxSafeInteger || kv.Int64() < jsMinSafeInteger {
				value = fmt.Sprintf("%d", value)
			}
		case model.Float64Type:
			value = kv.Float64()
		case model.BinaryType:
			value = kv.Binary()
		}

		out[i] = KeyValue{
			Key:   kv.Key,
			Type:  ValueType(strings.ToLower(kv.VType.String())),
			Value: value,
		}
	}
	return out
}

func (fromDomain) convertKeyValuesString(keyValues model.KeyValues) []KeyValue {
	out := make([]KeyValue, len(keyValues))
	for i, kv := range keyValues {
		out[i] = KeyValue{
			Key:   kv.Key,
			Type:  ValueType(strings.ToLower(kv.VType.String())),
			Value: kv.AsString(),
		}
	}
	return out
}

func (fd fromDomain) convertLogs(logs []model.Log) []Log {
	out := make([]Log, len(logs))
	for i, log := range logs {
		out[i] = Log{
			Timestamp: model.TimeAsEpochMicroseconds(log.Timestamp),
			Fields:    fd.convertKeyValuesFunc(log.Fields),
		}
	}
	return out
}

func (fd fromDomain) convertProcesses(processes map[string]*model.Process) map[ProcessID]Process {
	out := make(map[ProcessID]Process)
	for key, process := range processes {
		out[ProcessID(key)] = fd.convertProcess(process)
	}
	return out
}

func (fd fromDomain) convertProcess(process *model.Process) Process {
	return Process{
		ServiceName: process.ServiceName,
		Tags:        fd.convertKeyValuesFunc(process.Tags),
	}
}

// DependenciesFromDomain converts []model.DependencyLink into []json.DependencyLink format.
func DependenciesFromDomain(dependencyLinks []model.DependencyLink) []DependencyLink {
	retMe := make([]DependencyLink, 0, len(dependencyLinks))
	for _, dependencyLink := range dependencyLinks {
		retMe = append(
			retMe,
			DependencyLink{
				Parent:    dependencyLink.Parent,
				Child:     dependencyLink.Child,
				CallCount: dependencyLink.CallCount,
			},
		)
	}
	return retMe
}
