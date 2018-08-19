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

package json

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/pkg/errors"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/json"
)

// SpanToDomain converts json.Span with embedded Process into model.Span format.
func SpanToDomain(span *json.Span) (*model.Span, error) {
	return toDomain{}.spanToDomain(span)
}

type toDomain struct{}

func (td toDomain) spanToDomain(dbSpan *json.Span) (*model.Span, error) {
	tags, err := td.convertKeyValues(dbSpan.Tags)
	if err != nil {
		return nil, err
	}
	logs, err := td.convertLogs(dbSpan.Logs)
	if err != nil {
		return nil, err
	}
	refs, err := td.convertRefs(dbSpan.References)
	if err != nil {
		return nil, err
	}
	process, err := td.convertProcess(dbSpan.Process)
	if err != nil {
		return nil, err
	}
	traceID, err := model.TraceIDFromString(string(dbSpan.TraceID))
	if err != nil {
		return nil, err
	}

	spanIDInt, err := model.SpanIDFromString(string(dbSpan.SpanID))
	if err != nil {
		return nil, err
	}

	if dbSpan.ParentSpanID != "" {
		parentSpanID, err := model.SpanIDFromString(string(dbSpan.ParentSpanID))
		if err != nil {
			return nil, err
		}
		refs = model.MaybeAddParentSpanID(traceID, parentSpanID, refs)
	}

	span := &model.Span{
		TraceID:       traceID,
		SpanID:        model.NewSpanID(uint64(spanIDInt)),
		OperationName: dbSpan.OperationName,
		References:    refs,
		Flags:         model.Flags(uint32(dbSpan.Flags)),
		StartTime:     model.EpochMicrosecondsAsTime(dbSpan.StartTime),
		Duration:      model.MicrosecondsAsDuration(dbSpan.Duration),
		Tags:          tags,
		Logs:          logs,
		Process:       process,
		Incomplete:    dbSpan.Incomplete,
	}
	return span, nil
}

func (td toDomain) convertRefs(refs []json.Reference) ([]model.SpanRef, error) {
	retMe := make([]model.SpanRef, len(refs))
	for i, r := range refs {
		// There are some inconsistencies with ReferenceTypes, hence the hacky fix.
		var refType model.SpanRefType
		if r.RefType == json.ChildOf {
			refType = model.ChildOf
		} else if r.RefType == json.FollowsFrom {
			refType = model.FollowsFrom
		} else {
			return nil, fmt.Errorf("not a valid SpanRefType string %s", string(r.RefType))
		}

		traceID, err := model.TraceIDFromString(string(r.TraceID))
		if err != nil {
			return nil, err
		}

		spanID, err := strconv.ParseUint(string(r.SpanID), 16, 64)
		if err != nil {
			return nil, err
		}

		retMe[i] = model.SpanRef{
			RefType: refType,
			TraceID: traceID,
			SpanID:  model.NewSpanID(spanID),
		}
	}
	return retMe, nil
}

func (td toDomain) convertKeyValues(tags []json.KeyValue) ([]model.KeyValue, error) {
	retMe := make([]model.KeyValue, len(tags))
	for i := range tags {
		kv, err := td.convertKeyValue(&tags[i])
		if err != nil {
			return nil, err
		}
		retMe[i] = kv
	}
	return retMe, nil
}

// convertKeyValue expects the Value field to be string, because it only works
// as a reverse transformation after FromDomain() for ElasticSearch model.
// This method would not work on general JSON from the UI which may contain numeric values.
func (td toDomain) convertKeyValue(tag *json.KeyValue) (model.KeyValue, error) {
	if tag.Value == nil {
		return model.KeyValue{}, fmt.Errorf("invalid nil Value in %v", tag)
	}
	tagValue, ok := tag.Value.(string)
	if !ok {
		return model.KeyValue{}, fmt.Errorf("non-string Value of type %t in %v", tag.Value, tag)
	}
	switch tag.Type {
	case json.StringType:
		return model.String(tag.Key, tagValue), nil
	case json.BoolType:
		value, err := strconv.ParseBool(tagValue)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Bool(tag.Key, value), nil
	case json.Int64Type:
		value, err := strconv.ParseInt(tagValue, 10, 64)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Int64(tag.Key, value), nil
	case json.Float64Type:
		value, err := strconv.ParseFloat(tagValue, 64)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Float64(tag.Key, value), nil
	case json.BinaryType:
		value, err := hex.DecodeString(tagValue)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Binary(tag.Key, value), nil
	}
	return model.KeyValue{}, fmt.Errorf("not a valid ValueType string %s", string(tag.Type))
}

func (td toDomain) convertLogs(logs []json.Log) ([]model.Log, error) {
	retMe := make([]model.Log, len(logs))
	for i, l := range logs {
		fields, err := td.convertKeyValues(l.Fields)
		if err != nil {
			return nil, err
		}
		retMe[i] = model.Log{
			Timestamp: model.EpochMicrosecondsAsTime(l.Timestamp),
			Fields:    fields,
		}
	}
	return retMe, nil
}

func (td toDomain) convertProcess(process *json.Process) (*model.Process, error) {
	if process == nil {
		return nil, errors.New("Process is nil")
	}
	tags, err := td.convertKeyValues(process.Tags)
	if err != nil {
		return nil, err
	}
	return &model.Process{
		Tags:        tags,
		ServiceName: process.ServiceName,
	}, nil
}
