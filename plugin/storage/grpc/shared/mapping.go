// Copyright (c) 2018 The Jaeger Authors.
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

package shared

import (
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/proto"
)

func DependencyLinkSliceFromProto(p []*proto.DependencyLink) []model.DependencyLink {
	rs := make([]model.DependencyLink, len(p))

	for n, dep := range p {
		rs[n] = DependencyLinkFromProto(dep)
	}

	return rs
}

func DependencyLinkSliceToProto(ds []model.DependencyLink) []*proto.DependencyLink {
	rs := make([]*proto.DependencyLink, len(ds))

	for n, dep := range ds {
		rs[n] = DependencyLinkToProto(&dep)
	}

	return rs
}

func DependencyLinkToProto(dep *model.DependencyLink) *proto.DependencyLink {
	return &proto.DependencyLink{
		Parent:    dep.Parent,
		Child:     dep.Child,
		CallCount: dep.CallCount,
	}
}

func DependencyLinkFromProto(p *proto.DependencyLink) model.DependencyLink {
	return model.DependencyLink{
		Parent:    p.Parent,
		Child:     p.Child,
		CallCount: p.CallCount,
	}
}

func TraceProcessMappingSliceToProto(mappings []model.Trace_ProcessMapping) []*proto.TraceProcessMapping {
	rs := make([]*proto.TraceProcessMapping, len(mappings))

	for n, mapping := range mappings {
		rs[n] = TraceProcessMappingToProto(&mapping)
	}

	return rs
}

func TraceProcessMappingSliceFromProto(p []*proto.TraceProcessMapping) []model.Trace_ProcessMapping {
	rs := make([]model.Trace_ProcessMapping, len(p))

	for n, mapping := range p {
		rs[n] = TraceProcessMappingFromProto(mapping)
	}

	return rs
}

func TraceProcessMappingToProto(mapping *model.Trace_ProcessMapping) *proto.TraceProcessMapping {
	return &proto.TraceProcessMapping{
		ProcessId: mapping.ProcessID,
		Process:   ProcessToProto(&mapping.Process),
	}
}

func TraceProcessMappingFromProto(p *proto.TraceProcessMapping) model.Trace_ProcessMapping {
	return model.Trace_ProcessMapping{
		ProcessID: p.ProcessId,
		Process:   ProcessFromProto(p.Process),
	}
}

func ProcessToProto(process *model.Process) *proto.Process {
	return &proto.Process{
		ServiceName: process.ServiceName,
		Tags:        KeyValueSliceToProto(process.Tags),
	}
}

func ProcessFromProto(p *proto.Process) model.Process {
	return model.Process{
		ServiceName: p.ServiceName,
		Tags:        KeyValueSliceFromProto(p.Tags),
	}
}

func ProcessPtrFromProto(p *proto.Process) *model.Process {
	if p == nil {
		return nil
	}

	return &model.Process{
		ServiceName: p.ServiceName,
		Tags:        KeyValueSliceFromProto(p.Tags),
	}
}

func LogSliceToProto(ls []model.Log) []*proto.Log {
	rs := make([]*proto.Log, len(ls))

	for n, log := range ls {
		rs[n] = LogToProto(&log)
	}

	return rs
}

func LogSliceFromProto(p []*proto.Log) []model.Log {
	rs := make([]model.Log, len(p))

	for n, log := range p {
		rs[n] = LogFromProto(log)
	}

	return rs
}

func LogToProto(log *model.Log) *proto.Log {
	return &proto.Log{
		Timestamp: TimeToProto(log.Timestamp),
		Fields:    KeyValueSliceToProto(log.Fields),
	}
}

func LogFromProto(p *proto.Log) model.Log {
	return model.Log{
		Timestamp: TimeFromProto(p.Timestamp),
		Fields:    KeyValueSliceFromProto(p.Fields),
	}
}

func ValueTypeToProto(vt model.ValueType) proto.ValueType {
	switch vt {
	case model.ValueType_STRING:
		return proto.ValueType_ValueType_STRING
	case model.ValueType_BOOL:
		return proto.ValueType_ValueType_BOOL
	case model.ValueType_INT64:
		return proto.ValueType_ValueType_INT64
	case model.ValueType_FLOAT64:
		return proto.ValueType_ValueType_FLOAT64
	case model.ValueType_BINARY:
		return proto.ValueType_ValueType_BINARY
	default:
		panic("unreachable")
	}
}

func ValueTypeFromProto(p proto.ValueType) model.ValueType {
	switch p {
	case proto.ValueType_ValueType_STRING:
		return model.ValueType_STRING
	case proto.ValueType_ValueType_BOOL:
		return model.ValueType_BOOL
	case proto.ValueType_ValueType_INT64:
		return model.ValueType_INT64
	case proto.ValueType_ValueType_FLOAT64:
		return model.ValueType_FLOAT64
	case proto.ValueType_ValueType_BINARY:
		return model.ValueType_BINARY
	default:
		panic("unreachable")
	}
}

func KeyValueSliceToProto(kvs []model.KeyValue) []*proto.KeyValue {
	rs := make([]*proto.KeyValue, len(kvs))

	for n, kv := range kvs {
		rs[n] = KeyValueToProto(&kv)
	}

	return rs
}

func KeyValueSliceFromProto(p []*proto.KeyValue) []model.KeyValue {
	rs := make([]model.KeyValue, len(p))

	for n, kv := range p {
		rs[n] = KeyValueFromProto(kv)
	}

	return rs
}

func KeyValueToProto(kv *model.KeyValue) *proto.KeyValue {
	return &proto.KeyValue{
		Key:          kv.Key,
		ValueType:    ValueTypeToProto(kv.VType),
		StringValue:  kv.VStr,
		BoolValue:    kv.VBool,
		Int64Value:   kv.VInt64,
		Float64Value: kv.VFloat64,
		BinaryValue:  kv.VBinary,
	}
}

func KeyValueFromProto(p *proto.KeyValue) model.KeyValue {
	return model.KeyValue{
		Key:      p.Key,
		VType:    ValueTypeFromProto(p.ValueType),
		VStr:     p.StringValue,
		VBool:    p.BoolValue,
		VInt64:   p.Int64Value,
		VFloat64: p.Float64Value,
		VBinary:  p.BinaryValue,
	}
}

func DurationToProto(d time.Duration) *proto.Duration {
	nanos := d.Nanoseconds()
	secs := nanos / 1e9
	nanos -= secs * 1e9
	return &proto.Duration{
		Seconds: secs,
		Nanos:   int32(nanos),
	}
}

func DurationFromProto(p *proto.Duration) time.Duration {
	d := time.Duration(p.Seconds) * time.Second
	if p.Nanos != 0 {
		d += time.Duration(p.Nanos)
	}

	return d
}

func TimeToProto(t time.Time) *proto.Timestamp {
	seconds := t.Unix()
	return &proto.Timestamp{
		Seconds: seconds,
		Nanos:   int32(t.Sub(time.Unix(seconds, 0))),
	}
}

func TimeFromProto(p *proto.Timestamp) time.Time {
	return time.Unix(p.Seconds, int64(p.Nanos))
}

func SpanRefTypeToProto(spanRefType model.SpanRefType) proto.SpanRefType {
	switch spanRefType {
	case model.FollowsFrom:
		return proto.SpanRefType_SpanRefType_FOLLOWS_FROM
	case model.ChildOf:
		return proto.SpanRefType_SpanRefType_CHILD_OF
	default:
		panic("unreachable")
	}
}

func SpanRefTypeFromProto(p proto.SpanRefType) model.SpanRefType {
	switch p {
	case proto.SpanRefType_SpanRefType_CHILD_OF:
		return model.ChildOf
	case proto.SpanRefType_SpanRefType_FOLLOWS_FROM:
		return model.FollowsFrom
	default:
		panic("unreachable")
	}
}

func SpanRefSliceToProto(srs []model.SpanRef) []*proto.SpanRef {
	rs := make([]*proto.SpanRef, len(srs))

	for n, spanRef := range srs {
		rs[n] = SpanRefToProto(&spanRef)
	}

	return rs
}

func SpanRefSliceFromProto(p []*proto.SpanRef) []model.SpanRef {
	rs := make([]model.SpanRef, len(p))

	for n, spanRef := range p {
		rs[n] = SpanRefFromProto(spanRef)
	}

	return rs
}

func SpanRefToProto(spanRef *model.SpanRef) *proto.SpanRef {
	return &proto.SpanRef{
		TraceId: TraceIDToProto(&spanRef.TraceID),
		SpanId:  uint64(spanRef.SpanID),
		RefType: SpanRefTypeToProto(spanRef.RefType),
	}
}

func SpanRefFromProto(p *proto.SpanRef) model.SpanRef {
	return model.SpanRef{
		TraceID: TraceIDFromProto(p.TraceId),
		SpanID:  model.SpanID(p.SpanId),
		RefType: SpanRefTypeFromProto(p.RefType),
	}
}

func TraceIDToProto(tid *model.TraceID) *proto.TraceId {
	return &proto.TraceId{
		Low:  tid.Low,
		High: tid.High,
	}
}

func TraceIDFromProto(p *proto.TraceId) model.TraceID {
	return model.TraceID{
		Low:  p.Low,
		High: p.High,
	}
}

func TraceSliceToProto(ts []*model.Trace) []*proto.Trace {
	rs := make([]*proto.Trace, len(ts))

	for n, trace := range ts {
		rs[n] = TraceToProto(trace)
	}

	return rs
}

func TraceSliceFromProto(p []*proto.Trace) []*model.Trace {
	rs := make([]*model.Trace, len(p))

	for n, trace := range p {
		rs[n] = TraceFromProto(trace)
	}

	return rs
}

func TraceToProto(trace *model.Trace) *proto.Trace {
	return &proto.Trace{
		Spans:      SpanSliceToProto(trace.Spans),
		ProcessMap: TraceProcessMappingSliceToProto(trace.ProcessMap),
		Warnings:   trace.Warnings,
	}
}

func TraceFromProto(p *proto.Trace) *model.Trace {
	return &model.Trace{
		Spans:      SpanSliceFromProto(p.Spans),
		ProcessMap: TraceProcessMappingSliceFromProto(p.ProcessMap),
		Warnings:   p.Warnings,
	}
}

func SpanSliceToProto(spans []*model.Span) []*proto.Span {
	rs := make([]*proto.Span, len(spans))

	for n, span := range spans {
		rs[n] = SpanToProto(span)
	}

	return rs
}

func SpanSliceFromProto(p []*proto.Span) []*model.Span {
	rs := make([]*model.Span, len(p))

	for n, span := range p {
		rs[n] = SpanFromProto(span)
	}

	return rs
}

func SpanToProto(span *model.Span) *proto.Span {
	return &proto.Span{
		TraceId:       TraceIDToProto(&span.TraceID),
		SpanId:        uint64(span.SpanID),
		OperationName: span.OperationName,
		References:    SpanRefSliceToProto(span.References),
		Flags:         uint32(span.Flags),
		StartTime:     TimeToProto(span.StartTime),
		Duration:      DurationToProto(span.Duration),
		Tags:          KeyValueSliceToProto(span.Tags),
		Logs:          LogSliceToProto(span.Logs),
		Process:       ProcessToProto(span.Process),
		ProcessId:     span.ProcessID,
		Warnings:      span.Warnings,
	}
}

func SpanFromProto(p *proto.Span) *model.Span {
	return &model.Span{
		TraceID:       TraceIDFromProto(p.TraceId),
		SpanID:        model.SpanID(p.SpanId),
		OperationName: p.OperationName,
		References:    SpanRefSliceFromProto(p.References),
		Flags:         model.Flags(p.Flags),
		StartTime:     TimeFromProto(p.StartTime),
		Duration:      DurationFromProto(p.Duration),
		Tags:          KeyValueSliceFromProto(p.Tags),
		Logs:          LogSliceFromProto(p.Logs),
		Process:       ProcessPtrFromProto(p.Process),
		ProcessID:     p.ProcessId,
		Warnings:      p.Warnings,
	}
}
