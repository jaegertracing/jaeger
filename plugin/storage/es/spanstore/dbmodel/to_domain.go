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
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/jaegertracing/jaeger/model"
)

// NewToDomain creates ToDomain
func NewToDomain(tagDotReplacement string) ToDomain {
	return ToDomain{tagDotReplacement: tagDotReplacement}
}

// ToDomain is used to convert Span to model.Span
type ToDomain struct {
	tagDotReplacement string
}

// ReplaceDot replaces dot with dotReplacement
func (td ToDomain) ReplaceDot(k string) string {
	return strings.Replace(k, ".", td.tagDotReplacement, -1)
}

// ReplaceDotReplacement replaces dotReplacement with dot
func (td ToDomain) ReplaceDotReplacement(k string) string {
	return strings.Replace(k, td.tagDotReplacement, ".", -1)
}

// SpanToDomain converts db span into model Span
func (td ToDomain) SpanToDomain(dbSpan *Span) (*model.Span, error) {
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

	fieldTags, err := td.convertTagFields(dbSpan.Tag)
	if err != nil {
		return nil, err
	}
	tags = append(tags, fieldTags...)

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

func (td ToDomain) convertRefs(refs []Reference) ([]model.SpanRef, error) {
	retMe := make([]model.SpanRef, len(refs))
	for i, r := range refs {
		// There are some inconsistencies with ReferenceTypes, hence the hacky fix.
		var refType model.SpanRefType
		if r.RefType == ChildOf {
			refType = model.ChildOf
		} else if r.RefType == FollowsFrom {
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

func (td ToDomain) convertKeyValues(tags []KeyValue) ([]model.KeyValue, error) {
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

func (td ToDomain) convertTagFields(tagsMap map[string]interface{}) ([]model.KeyValue, error) {
	kvs := make([]model.KeyValue, len(tagsMap))
	i := 0
	for k, v := range tagsMap {
		tag, err := td.convertTagField(k, v)
		if err != nil {
			return nil, err
		}
		kvs[i] = tag
		i++
	}
	return kvs, nil
}

func (td ToDomain) convertTagField(k string, v interface{}) (model.KeyValue, error) {
	dKey := td.ReplaceDotReplacement(k)
	// The number is always a float64 therefore type assertion on int (v.(int/64/32)) does not work.
	// If 1.0, 2.0.. was stored as float it will be read as int
	if pInt, err := strconv.ParseInt(fmt.Sprintf("%v", v), 10, 64); err == nil {
		return model.Int64(k, pInt), nil
	}
	switch val := v.(type) {
	case float64:
		return model.Float64(dKey, val), nil
	case bool:
		return model.Bool(dKey, val), nil
	case string:
		return model.String(dKey, val), nil
	// the binary is never returned, ES returns it as string with base64 encoding
	case []byte:
		return model.Binary(dKey, val), nil
	default:
		return model.String("", ""), fmt.Errorf("invalid tag type in %+v", v)
	}
}

// convertKeyValue expects the Value field to be string, because it only works
// as a reverse transformation after FromDomain() for ElasticSearch model.
func (td ToDomain) convertKeyValue(tag *KeyValue) (model.KeyValue, error) {
	if tag.Value == nil {
		return model.KeyValue{}, fmt.Errorf("invalid nil Value in %v", tag)
	}
	tagValue, ok := tag.Value.(string)
	if !ok {
		return model.KeyValue{}, fmt.Errorf("non-string Value of type %t in %v", tag.Value, tag)
	}
	switch tag.Type {
	case StringType:
		return model.String(tag.Key, tagValue), nil
	case BoolType:
		value, err := strconv.ParseBool(tagValue)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Bool(tag.Key, value), nil
	case Int64Type:
		value, err := strconv.ParseInt(tagValue, 10, 64)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Int64(tag.Key, value), nil
	case Float64Type:
		value, err := strconv.ParseFloat(tagValue, 64)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Float64(tag.Key, value), nil
	case BinaryType:
		value, err := hex.DecodeString(tagValue)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Binary(tag.Key, value), nil
	}
	return model.KeyValue{}, fmt.Errorf("not a valid ValueType string %s", string(tag.Type))
}

func (td ToDomain) convertLogs(logs []Log) ([]model.Log, error) {
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

func (td ToDomain) convertProcess(process Process) (*model.Process, error) {
	tags, err := td.convertKeyValues(process.Tags)
	if err != nil {
		return nil, err
	}
	fieldTags, err := td.convertTagFields(process.Tag)
	if err != nil {
		return nil, err
	}
	tags = append(tags, fieldTags...)

	return &model.Process{
		Tags:        tags,
		ServiceName: process.ServiceName,
	}, nil
}
