// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"fmt"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/olivere/elastic/v7"
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

type TagQueryBuilder struct {
	dotReplacer dbmodel.DotReplacer
}

// NewTagQueryBuilder returns an instance of TagQueryBuilder
func NewTagQueryBuilder(dotReplacer dbmodel.DotReplacer) TagQueryBuilder {
	return TagQueryBuilder{dotReplacer: dotReplacer}
}

func (q *TagQueryBuilder) BuildTagQuery(k string, v string) elastic.Query {
	objectTagListLen := len(objectTagFieldList)
	queries := make([]elastic.Query, len(nestedTagFieldList)+objectTagListLen)
	kd := q.dotReplacer.ReplaceDot(k)
	for i := range objectTagFieldList {
		queries[i] = q.buildObjectQuery(objectTagFieldList[i], kd, v)
	}
	for i := range nestedTagFieldList {
		queries[i+objectTagListLen] = q.buildNestedQuery(nestedTagFieldList[i], k, v)
	}

	// but configuration can change over time
	return elastic.NewBoolQuery().Should(queries...)
}

func (*TagQueryBuilder) buildNestedQuery(field string, k string, v string) elastic.Query {
	keyField := fmt.Sprintf("%s.%s", field, tagKeyField)
	valueField := fmt.Sprintf("%s.%s", field, tagValueField)
	keyQuery := elastic.NewMatchQuery(keyField, k)
	valueQuery := elastic.NewRegexpQuery(valueField, v)
	tagBoolQuery := elastic.NewBoolQuery().Must(keyQuery, valueQuery)
	return elastic.NewNestedQuery(field, tagBoolQuery)
}

func (*TagQueryBuilder) buildObjectQuery(field string, k string, v string) elastic.Query {
	keyField := fmt.Sprintf("%s.%s", field, k)
	keyQuery := elastic.NewRegexpQuery(keyField, v)
	return elastic.NewBoolQuery().Must(keyQuery)
}
