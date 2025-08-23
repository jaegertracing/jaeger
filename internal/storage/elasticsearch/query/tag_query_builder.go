// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"fmt"

	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

const (
	objectTagsField        = "tag"
	objectProcessTagsField = "process.tag"
	nestedTagsField        = "tags"
	nestedProcessTagsField = "process.tags"
	nestedLogFieldsField   = "logs.fields"
	tagKeyField            = "key"
	tagValueField          = "value"
)

var (
	objectTagFieldList = []string{objectTagsField, objectProcessTagsField}
	nestedTagFieldList = []string{nestedTagsField, nestedProcessTagsField, nestedLogFieldsField}
)

// TagQueryBuilder builds ES queries for tag key-value filters.
type TagQueryBuilder struct {
	dotReplacer dbmodel.DotReplacer
}

// NewTagQueryBuilder returns an instance of TagQueryBuilder.
func NewTagQueryBuilder(dotReplacer dbmodel.DotReplacer) TagQueryBuilder {
	return TagQueryBuilder{dotReplacer: dotReplacer}
}

// BuildTagQuery constructs a query that matches spans containing a tag with the
// given key and value.
func (q *TagQueryBuilder) BuildTagQuery(k, v string) Query {
	objectTagListLen := len(objectTagFieldList)
	queries := make([]Query, len(nestedTagFieldList)+objectTagListLen)
	kd := q.dotReplacer.ReplaceDot(k)
	for i := range objectTagFieldList {
		queries[i] = q.buildObjectQuery(objectTagFieldList[i], kd, v)
	}
	for i := range nestedTagFieldList {
		queries[i+objectTagListLen] = q.buildNestedQuery(nestedTagFieldList[i], k, v)
	}
	return NewBoolQuery().Should(queries...)
}

func (*TagQueryBuilder) buildNestedQuery(field, k, v string) Query {
	keyField := fmt.Sprintf("%s.%s", field, tagKeyField)
	valueField := fmt.Sprintf("%s.%s", field, tagValueField)
	keyQuery := NewMatchQuery(keyField, k)
	valueQuery := NewRegexpQuery(valueField, v)
	tagBoolQuery := NewBoolQuery().Must(keyQuery, valueQuery)
	return NewNestedQuery(field, tagBoolQuery)
}

func (*TagQueryBuilder) buildObjectQuery(field, k, v string) Query {
	keyField := fmt.Sprintf("%s.%s", field, k)
	keyQuery := NewRegexpQuery(keyField, v)
	return NewBoolQuery().Must(keyQuery)
}
