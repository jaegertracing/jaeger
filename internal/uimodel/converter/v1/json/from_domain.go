// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"fmt"
	"strings"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/uimodel"
)

const (
	jsMaxSafeInteger = int64(1)<<53 - 1
	jsMinSafeInteger = -jsMaxSafeInteger
)

// FromDomain converts model.Trace into json.Trace format.
// It assumes that the domain model is valid, namely that all enums
// have valid values, so that it does not need to check for errors.
func FromDomain(trace *model.Trace) *uimodel.Trace {
	fd := fromDomain{}
	fd.convertKeyValuesFunc = fd.convertKeyValues
	return fd.fromDomain(trace)
}

// FromDomainEmbedProcess converts model.Span into json.Span format.
// This format includes a ParentSpanID and an embedded Process.
func FromDomainEmbedProcess(span *model.Span) *uimodel.Span {
	fd := fromDomain{}
	fd.convertKeyValuesFunc = fd.convertKeyValuesString
	return fd.convertSpanEmbedProcess(span)
}

type fromDomain struct {
	convertKeyValuesFunc func(keyValues model.KeyValues) []uimodel.KeyValue
}

func (fd fromDomain) fromDomain(trace *model.Trace) *uimodel.Trace {
	jSpans := make([]uimodel.Span, len(trace.Spans))
	processes := &processHashtable{}
	var traceID uimodel.TraceID
	for i, span := range trace.Spans {
		if i == 0 {
			traceID = uimodel.TraceID(span.TraceID.String())
		}
		processID := uimodel.ProcessID(processes.getKey(span.Process))
		jSpans[i] = fd.convertSpan(span, processID)
	}
	jTrace := &uimodel.Trace{
		TraceID:   traceID,
		Spans:     jSpans,
		Processes: fd.convertProcesses(processes.getMapping()),
		Warnings:  trace.Warnings,
	}
	return jTrace
}

func (fd fromDomain) convertSpanInternal(span *model.Span) uimodel.Span {
	return uimodel.Span{
		TraceID:       uimodel.TraceID(span.TraceID.String()),
		SpanID:        uimodel.SpanID(span.SpanID.String()),
		Flags:         uint32(span.Flags),
		OperationName: span.OperationName,
		StartTime:     model.TimeAsEpochMicroseconds(span.StartTime),
		Duration:      model.DurationAsMicroseconds(span.Duration),
		Tags:          fd.convertKeyValuesFunc(span.Tags),
		Logs:          fd.convertLogs(span.Logs),
	}
}

func (fd fromDomain) convertSpan(span *model.Span, processID uimodel.ProcessID) uimodel.Span {
	s := fd.convertSpanInternal(span)
	s.ProcessID = processID
	s.Warnings = span.Warnings
	s.References = fd.convertReferences(span)
	return s
}

func (fd fromDomain) convertSpanEmbedProcess(span *model.Span) *uimodel.Span {
	s := fd.convertSpanInternal(span)
	process := fd.convertProcess(span.Process)
	s.Process = &process
	s.References = fd.convertReferences(span)
	return &s
}

func (fd fromDomain) convertReferences(span *model.Span) []uimodel.Reference {
	out := make([]uimodel.Reference, 0, len(span.References))
	for _, ref := range span.References {
		out = append(out, uimodel.Reference{
			RefType: fd.convertRefType(ref.RefType),
			TraceID: uimodel.TraceID(ref.TraceID.String()),
			SpanID:  uimodel.SpanID(ref.SpanID.String()),
		})
	}
	return out
}

func (fromDomain) convertRefType(refType model.SpanRefType) uimodel.ReferenceType {
	if refType == model.FollowsFrom {
		return uimodel.FollowsFrom
	}
	return uimodel.ChildOf
}

func (fromDomain) convertKeyValues(keyValues model.KeyValues) []uimodel.KeyValue {
	out := make([]uimodel.KeyValue, len(keyValues))
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

		out[i] = uimodel.KeyValue{
			Key:   kv.Key,
			Type:  uimodel.ValueType(strings.ToLower(kv.VType.String())),
			Value: value,
		}
	}
	return out
}

func (fromDomain) convertKeyValuesString(keyValues model.KeyValues) []uimodel.KeyValue {
	out := make([]uimodel.KeyValue, len(keyValues))
	for i, kv := range keyValues {
		out[i] = uimodel.KeyValue{
			Key:   kv.Key,
			Type:  uimodel.ValueType(strings.ToLower(kv.VType.String())),
			Value: kv.AsString(),
		}
	}
	return out
}

func (fd fromDomain) convertLogs(logs []model.Log) []uimodel.Log {
	out := make([]uimodel.Log, len(logs))
	for i, log := range logs {
		out[i] = uimodel.Log{
			Timestamp: model.TimeAsEpochMicroseconds(log.Timestamp),
			Fields:    fd.convertKeyValuesFunc(log.Fields),
		}
	}
	return out
}

func (fd fromDomain) convertProcesses(processes map[string]*model.Process) map[uimodel.ProcessID]uimodel.Process {
	out := make(map[uimodel.ProcessID]uimodel.Process)
	for key, process := range processes {
		out[uimodel.ProcessID(key)] = fd.convertProcess(process)
	}
	return out
}

func (fd fromDomain) convertProcess(process *model.Process) uimodel.Process {
	return uimodel.Process{
		ServiceName: process.ServiceName,
		Tags:        fd.convertKeyValuesFunc(process.Tags),
	}
}

// DependenciesFromDomain converts []model.DependencyLink into []json.DependencyLink format.
func DependenciesFromDomain(dependencyLinks []model.DependencyLink) []uimodel.DependencyLink {
	retMe := make([]uimodel.DependencyLink, 0, len(dependencyLinks))
	for _, dependencyLink := range dependencyLinks {
		retMe = append(
			retMe,
			uimodel.DependencyLink{
				Parent:    dependencyLink.Parent,
				Child:     dependencyLink.Child,
				CallCount: dependencyLink.CallCount,
			},
		)
	}
	return retMe
}
