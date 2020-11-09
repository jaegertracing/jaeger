// Copyright (c) 2020 The Jaeger Authors.
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

package esspanreader

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	getServicesAggregation = `{
    "serviceName": {
      "terms": {
        "field": "serviceName",
        "size": %d
      }
    }
  }
`
	getOperationsAggregation = `{
    "operationName": {
      "terms": {
        "field": "operationName",
        "size": %d
      }
    }
  }
`
	findTraceIDsAggregation = `{
    "traceID": {
      "aggs": {
        "startTime": {
          "max": {
            "field": "startTime"
          }
        }
      },
      "terms": {
        "field": "traceID",
        "size": %d,
        "order": {
          "startTime": "desc"
        }
      }
    }
  }
`
)

var (
	defaultMaxDuration = model.DurationAsMicroseconds(time.Hour * 24)
	objectTagFieldList = []string{objectTagsField, objectProcessTagsField}
	nestedTagFieldList = []string{nestedTagsField, nestedProcessTagsField, nestedLogFieldsField}
)

func buildDurationQuery(durationMin time.Duration, durationMax time.Duration, query esclient.Query) {
	minDurationMicros := model.DurationAsMicroseconds(durationMin)
	maxDurationMicros := defaultMaxDuration
	if durationMax != 0 {
		maxDurationMicros = model.DurationAsMicroseconds(durationMax)
	}
	query.BoolQuery[esclient.Must] = append(query.BoolQuery[esclient.Must],
		esclient.BoolQuery{
			RangeQueries: map[string]esclient.RangeQuery{durationField: {GTE: minDurationMicros, LTE: maxDurationMicros}}})
}

func addStartTimeQuery(startTimeMin time.Time, startTimeMax time.Time, query esclient.Query) {
	minStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMin)
	maxStartTimeMicros := model.TimeAsEpochMicroseconds(startTimeMax)
	query.BoolQuery[esclient.Must] = append(query.BoolQuery[esclient.Must], esclient.BoolQuery{RangeQueries: map[string]esclient.RangeQuery{startTimeField: {GTE: minStartTimeMicros, LTE: maxStartTimeMicros}}})
}

func addServiceNameQuery(serviceName string, query esclient.Query) {
	query.BoolQuery[esclient.Must] = append(query.BoolQuery[esclient.Must], esclient.BoolQuery{Term: map[string]string{processServiceNameField: serviceName}})
}

func addOperationNameQuery(operationName string, query esclient.Query) {
	query.BoolQuery[esclient.Must] = append(query.BoolQuery[esclient.Must], esclient.BoolQuery{Term: map[string]string{operationNameField: operationName}})
}

func addTagQuery(converter dbmodel.ToDomain, tags map[string]string, query esclient.Query) {
	if len(tags) == 0 {
		return
	}

	tagQueries := esclient.BoolQuery{BoolQuery: map[esclient.BoolQueryType][]esclient.BoolQuery{}}
	for i := range objectTagFieldList {
		addObjectQuery(converter, objectTagFieldList[i], tags, tagQueries)
	}
	for i := range nestedTagFieldList {
		addNestedQuery(nestedTagFieldList[i], tags, tagQueries)
	}
	query.BoolQuery[esclient.Must] = append(query.BoolQuery[esclient.Must], tagQueries)
}

func addObjectQuery(converter dbmodel.ToDomain, field string, tags map[string]string, query esclient.BoolQuery) {
	for k, v := range tags {
		kd := converter.ReplaceDot(k)
		keyField := fmt.Sprintf("%s.%s", field, kd)
		query.BoolQuery[esclient.Should] = append(query.BoolQuery[esclient.Should],
			esclient.BoolQuery{BoolQuery: map[esclient.BoolQueryType][]esclient.BoolQuery{
				esclient.Must: {
					{Regexp: map[string]esclient.TermQuery{keyField: {Value: v}}},
				},
			}},
		)
	}
}

func addNestedQuery(nestedField string, tags map[string]string, query esclient.BoolQuery) {
	keyField := fmt.Sprintf("%s.%s", nestedField, tagKeyField)
	valueField := fmt.Sprintf("%s.%s", nestedField, tagValueField)
	for k, v := range tags {
		nestedQuery := &esclient.NestedQuery{
			Path: nestedField,
			Query: esclient.Query{
				BoolQuery: map[esclient.BoolQueryType][]esclient.BoolQuery{
					esclient.Must: {
						{MatchQueries: map[string]esclient.MatchQuery{keyField: {Query: k}}},
						{Regexp: map[string]esclient.TermQuery{valueField: {Value: v}}},
					},
				},
			},
		}
		query.BoolQuery[esclient.Should] = append(query.BoolQuery[esclient.Should],
			esclient.BoolQuery{
				Nested: nestedQuery,
			})
	}
}

func findTraceIDsQuery(converter dbmodel.ToDomain, query *spanstore.TraceQueryParameters) esclient.Query {
	q := esclient.Query{}
	q.BoolQuery = map[esclient.BoolQueryType][]esclient.BoolQuery{}
	// add startTime query
	addStartTimeQuery(query.StartTimeMin, query.StartTimeMax, q)
	// add duration query
	if query.DurationMax != 0 || query.DurationMin != 0 {
		buildDurationQuery(query.DurationMin, query.DurationMax, q)
	}
	// add process.serviceName query
	if query.ServiceName != "" {
		addServiceNameQuery(query.ServiceName, q)
	}
	// add operationName query
	if query.OperationName != "" {
		addOperationNameQuery(query.OperationName, q)
	}
	// add tag query
	addTagQuery(converter, query.Tags, q)
	return q
}

func traceIDQuery(traceID model.TraceID) *esclient.Query {
	traceIDStr := traceID.String()
	return &esclient.Query{Term: &esclient.Terms{
		traceIDField: esclient.TermQuery{
			Value: traceIDStr,
		},
	}}
}

func findTraceIDsSearchBody(converter dbmodel.ToDomain, query *spanstore.TraceQueryParameters) esclient.SearchBody {
	q := findTraceIDsQuery(converter, query)
	aggs := fmt.Sprintf(findTraceIDsAggregation, query.NumTraces)
	return esclient.SearchBody{
		Aggregations: json.RawMessage(aggs),
		Query:        &q,
	}
}

func getServicesSearchBody(maxDocCount int) esclient.SearchBody {
	aggs := fmt.Sprintf(getServicesAggregation, maxDocCount)
	return esclient.SearchBody{
		Aggregations: json.RawMessage(aggs),
	}
}

func getOperationsSearchBody(serviceName string, maxDocCount int) esclient.SearchBody {
	aggs := fmt.Sprintf(getOperationsAggregation, maxDocCount)
	return esclient.SearchBody{
		Aggregations: json.RawMessage(aggs),
		Query: &esclient.Query{
			Term: &esclient.Terms{
				serviceNameField: esclient.TermQuery{Value: serviceName},
			},
		},
	}
}
