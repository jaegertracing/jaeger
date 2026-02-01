// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package processor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

// TagProcessor handles tag transformations for span storage
type TagProcessor struct {
	dotReplacer dbmodel.DotReplacer
}

// NewTagProcessor creates a new tag processor
func NewTagProcessor(dotReplacer dbmodel.DotReplacer) *TagProcessor {
	return &TagProcessor{dotReplacer: dotReplacer}
}

// MergeAllNestedAndElevatedTagsOfSpan merges nested and elevated tags for both span and process tags
func (tp *TagProcessor) MergeAllNestedAndElevatedTagsOfSpan(span *dbmodel.Span) {
	processTags := tp.MergeNestedAndElevatedTags(span.Process.Tags, span.Process.Tag)
	span.Process.Tags = processTags
	spanTags := tp.MergeNestedAndElevatedTags(span.Tags, span.Tag)
	span.Tags = spanTags
}

// MergeNestedAndElevatedTags merges nested tags array with elevated tags map
func (tp *TagProcessor) MergeNestedAndElevatedTags(nestedTags []dbmodel.KeyValue, elevatedTags map[string]any) []dbmodel.KeyValue {
	mergedTags := make([]dbmodel.KeyValue, 0, len(nestedTags)+len(elevatedTags))
	mergedTags = append(mergedTags, nestedTags...)
	for k, v := range elevatedTags {
		kv := tp.convertTagField(k, v)
		mergedTags = append(mergedTags, kv)
		delete(elevatedTags, k)
	}
	return mergedTags
}

// convertTagField converts a tag field from map representation to KeyValue with proper type detection
func (tp *TagProcessor) convertTagField(k string, v any) dbmodel.KeyValue {
	dKey := tp.dotReplacer.ReplaceDotReplacement(k)
	kv := dbmodel.KeyValue{
		Key:   dKey,
		Value: v,
	}
	switch val := v.(type) {
	case int64:
		kv.Type = dbmodel.Int64Type
	case float64:
		kv.Type = dbmodel.Float64Type
	case bool:
		kv.Type = dbmodel.BoolType
	case string:
		kv.Type = dbmodel.StringType
	// the binary is never returned, ES returns it as string with base64 encoding
	case []byte:
		kv.Type = dbmodel.BinaryType
	// in spans are decoded using json.UseNumber() to preserve the type
	// however note that float(1) will be parsed as int as ES does not store decimal point
	case json.Number:
		n, err := val.Int64()
		if err == nil {
			kv.Value = n
			kv.Type = dbmodel.Int64Type
		} else {
			f, err := val.Float64()
			if err != nil {
				return dbmodel.KeyValue{
					Key:   dKey,
					Value: fmt.Sprintf("invalid tag type in %+v: %s", v, err.Error()),
					Type:  dbmodel.StringType,
				}
			}
			kv.Value = f
			kv.Type = dbmodel.Float64Type
		}
	default:
		return dbmodel.KeyValue{
			Key:   dKey,
			Value: fmt.Sprintf("invalid tag type in %+v", v),
			Type:  dbmodel.StringType,
		}
	}
	return kv
}

// SplitElevatedTags splits tags into nested tags and elevated field tags based on configuration
func (*TagProcessor) SplitElevatedTags(keyValues []dbmodel.KeyValue, allTagsAsFields bool, tagKeysAsFields map[string]bool, tagDotReplacement string) ([]dbmodel.KeyValue, map[string]any) {
	if !allTagsAsFields && len(tagKeysAsFields) == 0 {
		return keyValues, nil
	}
	var tagsMap map[string]any
	var kvs []dbmodel.KeyValue
	for _, kv := range keyValues {
		if kv.Type != dbmodel.BinaryType && (allTagsAsFields || tagKeysAsFields[kv.Key]) {
			if tagsMap == nil {
				tagsMap = map[string]any{}
			}
			tagsMap[strings.ReplaceAll(kv.Key, ".", tagDotReplacement)] = kv.Value
		} else {
			kvs = append(kvs, kv)
		}
	}
	if kvs == nil {
		kvs = make([]dbmodel.KeyValue, 0)
	}
	return kvs, tagsMap
}
