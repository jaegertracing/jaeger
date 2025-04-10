// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

// NewToDomain creates ToDomain
func NewToDomain(tagDotReplacement string) ToDomain {
	return ToDomain{dotReplacer: dbmodel.NewDotReplacer(tagDotReplacement)}
}

// ToDomain is used to convert Span to model.Span
type ToDomain struct {
	dotReplacer dbmodel.DotReplacer
}

// SpanToDomain converts db span into model Span
func (td ToDomain) SpanToDomain(dbSpan *dbmodel.Span) (*model.Span, error) {
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
	}
	return span, nil
}

func (ToDomain) convertRefs(refs []dbmodel.Reference) ([]model.SpanRef, error) {
	retMe := make([]model.SpanRef, len(refs))
	for i, r := range refs {
		// There are some inconsistencies with ReferenceTypes, hence the hacky fix.
		var refType model.SpanRefType
		switch r.RefType {
		case dbmodel.ChildOf:
			refType = model.ChildOf
		case dbmodel.FollowsFrom:
			refType = model.FollowsFrom
		default:
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

func (td ToDomain) convertKeyValues(tags []dbmodel.KeyValue) ([]model.KeyValue, error) {
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

func (td ToDomain) convertTagFields(tagsMap map[string]any) ([]model.KeyValue, error) {
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

func (td ToDomain) convertTagField(k string, v any) (model.KeyValue, error) {
	dKey := td.dotReplacer.ReplaceDotReplacement(k)
	switch val := v.(type) {
	case int64:
		return model.Int64(dKey, val), nil
	case float64:
		return model.Float64(dKey, val), nil
	case bool:
		return model.Bool(dKey, val), nil
	case string:
		return model.String(dKey, val), nil
	// the binary is never returned, ES returns it as string with base64 encoding
	case []byte:
		return model.Binary(dKey, val), nil
	// in spans are decoded using json.UseNumber() to preserve the type
	// however note that float(1) will be parsed as int as ES does not store decimal point
	case json.Number:
		n, err := val.Int64()
		if err == nil {
			return model.Int64(dKey, n), nil
		}
		f, err := val.Float64()
		if err == nil {
			return model.Float64(dKey, f), nil
		}
		return model.String("", ""), fmt.Errorf("invalid tag type in %+v: %w", v, err)
	default:
		return model.String("", ""), fmt.Errorf("invalid tag type in %+v", v)
	}
}

// convertKeyValue expects the Value field to be string, because it only works
// as a reverse transformation after FromDomain() for ElasticSearch model.
func (td ToDomain) convertKeyValue(tag *dbmodel.KeyValue) (model.KeyValue, error) {
	if tag.Value == nil {
		return model.KeyValue{}, fmt.Errorf("invalid nil Value in %v", tag)
	}
	tagValue, ok := tag.Value.(string)
	if !ok {
		switch tag.Type {
		case dbmodel.Int64Type, dbmodel.Float64Type:
			kv, err := td.fromDBNumber(tag)
			if err != nil {
				return model.KeyValue{}, err
			}
			return kv, nil
		case dbmodel.BoolType:
			if boolVal, ok := tag.Value.(bool); ok {
				return model.Bool(tag.Key, boolVal), nil
			}
			return model.KeyValue{}, invalidValueErr(tag)
		// string and binary values should always be of string type
		default:
			return model.KeyValue{}, invalidValueErr(tag)
		}
	}
	switch tag.Type {
	case dbmodel.StringType:
		return model.String(tag.Key, tagValue), nil
	case dbmodel.BoolType:
		value, err := strconv.ParseBool(tagValue)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Bool(tag.Key, value), nil
	case dbmodel.Int64Type:
		value, err := strconv.ParseInt(tagValue, 10, 64)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Int64(tag.Key, value), nil
	case dbmodel.Float64Type:
		value, err := strconv.ParseFloat(tagValue, 64)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Float64(tag.Key, value), nil
	case dbmodel.BinaryType:
		value, err := hex.DecodeString(tagValue)
		if err != nil {
			return model.KeyValue{}, err
		}
		return model.Binary(tag.Key, value), nil
	}
	return model.KeyValue{}, fmt.Errorf("not a valid ValueType string %s", string(tag.Type))
}

func (ToDomain) fromDBNumber(kv *dbmodel.KeyValue) (model.KeyValue, error) {
	if kv.Type == dbmodel.Int64Type {
		switch v := kv.Value.(type) {
		case int64:
			return model.Int64(kv.Key, v), nil
		// This case is very much possible as JSON converts every number to float64
		case float64:
			return model.Int64(kv.Key, int64(v)), nil
		case json.Number:
			n, err := v.Int64()
			if err == nil {
				return model.Int64(kv.Key, n), nil
			}
		default:
			return model.KeyValue{}, invalidValueErr(kv)
		}
	} else if kv.Type == dbmodel.Float64Type {
		switch v := kv.Value.(type) {
		case float64:
			return model.Float64(kv.Key, v), nil
		case json.Number:
			n, err := v.Float64()
			if err == nil {
				return model.Float64(kv.Key, n), nil
			}
		default:
			return model.KeyValue{}, invalidValueErr(kv)
		}
	}
	return model.KeyValue{}, fmt.Errorf("not a valid number ValueType %s", string(kv.Type))
}

func invalidValueErr(kv *dbmodel.KeyValue) error {
	return fmt.Errorf("invalid %s type in %+v", string(kv.Type), kv.Value)
}

func (td ToDomain) convertLogs(logs []dbmodel.Log) ([]model.Log, error) {
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

func (td ToDomain) convertProcess(process dbmodel.Process) (*model.Process, error) {
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
