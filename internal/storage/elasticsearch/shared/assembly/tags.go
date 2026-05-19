// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package assembly

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

// MergeAllNestedAndElevatedTagsOfSpan merges nested and elevated tags
// for both the span and its process.
func MergeAllNestedAndElevatedTagsOfSpan(
	span *dbmodel.Span,
	dotReplacer dbmodel.DotReplacer,
) {
	processTags := MergeNestedAndElevatedTags(span.Process.Tags, span.Process.Tag, dotReplacer)
	span.Process.Tags = processTags
	spanTags := MergeNestedAndElevatedTags(span.Tags, span.Tag, dotReplacer)
	span.Tags = spanTags
}

// MergeNestedAndElevatedTags merges nestedTags and elevatedTags into one slice.
// MergeNestedAndElevatedTags merges nestedTags and elevatedTags into one slice.
// IMPORTANT: This function modifies the elevatedTags map by deleting entries as they are merged.
// This is intentional behavior to clear the elevated tags after merging them into the nested format.
func MergeNestedAndElevatedTags(
	nestedTags []dbmodel.KeyValue,
	elevatedTags map[string]any,
	dotReplacer dbmodel.DotReplacer,
) []dbmodel.KeyValue {
	mergedTags := make([]dbmodel.KeyValue, 0, len(nestedTags)+len(elevatedTags))
	mergedTags = append(mergedTags, nestedTags...)
	for k, v := range elevatedTags {
		kv := ConvertTagField(k, v, dotReplacer)
		mergedTags = append(mergedTags, kv)
		delete(elevatedTags, k)
	}
	return mergedTags
}

// ConvertTagField converts a raw key-value pair into a dbmodel.KeyValue
// with proper type detection.
func ConvertTagField(
	k string,
	v any,
	dotReplacer dbmodel.DotReplacer,
) dbmodel.KeyValue {
	dKey := dotReplacer.ReplaceDotReplacement(k)
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

// SplitElevatedTags splits keyValues into nested tags and elevated field tags.
func SplitElevatedTags(
	keyValues []dbmodel.KeyValue,
	allTagsAsFields bool,
	tagKeysAsFields map[string]bool,
	tagDotReplacement string,
) ([]dbmodel.KeyValue, map[string]any) {
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

// ConvertNestedTagsToFieldTags converts span and process nested tags
// to field tags in place.
func ConvertNestedTagsToFieldTags(
	span *dbmodel.Span,
	allTagsAsFields bool,
	tagKeysAsFields map[string]bool,
	tagDotReplacement string,
) {
	processNestedTags, processFieldTags := SplitElevatedTags(span.Process.Tags, allTagsAsFields, tagKeysAsFields, tagDotReplacement)
	span.Process.Tags = processNestedTags
	span.Process.Tag = processFieldTags
	nestedTags, fieldTags := SplitElevatedTags(span.Tags, allTagsAsFields, tagKeysAsFields, tagDotReplacement)
	span.Tags = nestedTags
	span.Tag = fieldTags
}
