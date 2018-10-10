// Copyright (c) 2018 Uber Technologies, Inc.
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
	"strings"

	"github.com/jaegertracing/jaeger/model"
)

// NewFromDomain creates FromDomain used to convert model span to db span
func NewFromDomain(allTagsAsObject bool, tagKeysAsFields []string, tagDotReplacement string) FromDomain {
	tags := map[string]bool{}
	for _, k := range tagKeysAsFields {
		tags[k] = true
	}
	return FromDomain{allTagsAsFields: allTagsAsObject, tagKeysAsFields: tags, tagDotReplacement: tagDotReplacement}
}

// FromDomain is used to convert model span to db span
type FromDomain struct {
	allTagsAsFields   bool
	tagKeysAsFields   map[string]bool
	tagDotReplacement string
}

// FromDomainEmbedProcess converts model.Span into json.Span format.
// This format includes a ParentSpanID and an embedded Process.
func (fd FromDomain) FromDomainEmbedProcess(span *model.Span) *Span {
	return fd.convertSpanEmbedProcess(span)
}

func (fd FromDomain) convertSpanInternal(span *model.Span) Span {
	tags, tagsMap := fd.convertKeyValuesString(span.Tags)
	return Span{
		TraceID:         TraceID(span.TraceID.String()),
		SpanID:          SpanID(span.SpanID.String()),
		Flags:           uint32(span.Flags),
		OperationName:   span.OperationName,
		StartTime:       model.TimeAsEpochMicroseconds(span.StartTime),
		StartTimeMillis: model.TimeAsEpochMicroseconds(span.StartTime) / 1000,
		Duration:        model.DurationAsMicroseconds(span.Duration),
		Tags:            tags,
		Tag:             tagsMap,
		Logs:            fd.convertLogs(span.Logs),
	}
}

func (fd FromDomain) convertSpanEmbedProcess(span *model.Span) *Span {
	s := fd.convertSpanInternal(span)
	s.Process = fd.convertProcess(span.Process)
	s.References = fd.convertReferences(span)
	return &s
}

func (fd FromDomain) convertReferences(span *model.Span) []Reference {
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

func (fd FromDomain) convertRefType(refType model.SpanRefType) ReferenceType {
	if refType == model.FollowsFrom {
		return FollowsFrom
	}
	return ChildOf
}

func (fd FromDomain) convertKeyValuesString(keyValues model.KeyValues) ([]KeyValue, map[string]interface{}) {
	var tagsMap map[string]interface{}
	var kvs []KeyValue
	for _, kv := range keyValues {
		if kv.GetVType() != model.BinaryType && (fd.allTagsAsFields || fd.tagKeysAsFields[kv.Key]) {
			if tagsMap == nil {
				tagsMap = map[string]interface{}{}
			}
			tagsMap[strings.Replace(kv.Key, ".", fd.tagDotReplacement, -1)] = kv.Value()
		} else {
			kvs = append(kvs, KeyValue{
				Key:   kv.Key,
				Type:  ValueType(strings.ToLower(kv.VType.String())),
				Value: kv.AsString(),
			})
		}
	}
	if kvs == nil {
		kvs = make([]KeyValue, 0)
	}
	return kvs, tagsMap
}

func (fd FromDomain) convertLogs(logs []model.Log) []Log {
	out := make([]Log, len(logs))
	for i, log := range logs {
		var kvs []KeyValue
		for _, kv := range log.Fields {
			kvs = append(kvs, KeyValue{
				Key:   kv.Key,
				Type:  ValueType(strings.ToLower(kv.VType.String())),
				Value: kv.AsString(),
			})
		}
		out[i] = Log{
			Timestamp: model.TimeAsEpochMicroseconds(log.Timestamp),
			Fields:    kvs,
		}
	}
	return out
}

func (fd FromDomain) convertProcess(process *model.Process) Process {
	tags, tagsMap := fd.convertKeyValuesString(process.Tags)
	return Process{
		ServiceName: process.ServiceName,
		Tags:        tags,
		Tag:         tagsMap,
	}
}
