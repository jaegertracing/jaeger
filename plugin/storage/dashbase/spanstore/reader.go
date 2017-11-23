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

package dashbase

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
	"net/http"
	"github.com/bitly/go-simplejson"
	"reflect"
	"github.com/spf13/cast"
	"fmt"
	"net/url"
	"strings"
)

// SpanReader can query for and load traces from ElasticSearch
type SpanReader struct {
	ctx         context.Context
	ServiceHost string
	logger      *zap.Logger
	maxLookback time.Duration
	// The age of the oldest service/operation we will look for. Because indices in ElasticSearch are by day,
	// this will be rounded down to UTC 00:00 of that day.
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(host string, logger *zap.Logger, maxLookback time.Duration, metricsFactory metrics.Factory) spanstore.Reader {
	//todo: support maxLookback
	return storageMetrics.NewReadMetricsDecorator(newSpanReader(host, logger, maxLookback), metricsFactory)
}

func newSpanReader(host string, logger *zap.Logger, maxLookback time.Duration) *SpanReader {
	ctx := context.Background()
	return &SpanReader{
		ctx:         ctx,
		ServiceHost: host,
		logger:      logger,
		maxLookback: maxLookback,
	}
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(traceID model.TraceID) (*model.Trace, error) {
	trace, err := s.getTrace(traceID.String())
	if err != nil {
		return nil, err
	}
	return trace, nil
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices() ([]string, error) {
	services := []string{}
	resp, err := http.Get("http://localhost:9876/v1/sql?sql=SELECT%20TOPN('ServiceName')%20last%2030%20day")
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	data, err := simplejson.NewFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	//a, err := data.GetPath("request.aggregations").Map()
	a, err := data.GetPath("request", "aggregations").Map()
	keys := reflect.ValueOf(a).MapKeys()
	if len(keys) != 1 {
		return nil, errors.New("aggregations fields get fail")
	}

	aggs := data.GetPath("aggregations", keys[0].String(), "facets").MustArray()

	for _, agg := range aggs {
		value, err := cast.ToStringMapE(agg)
		if err != nil {
			return nil, err
		}
		service_name := cast.ToString(value["value"])
		services = append(services, service_name)

	}

	return services, nil
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(service string) ([]string, error) {
	return []string{}, nil

}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
	ids, err := s.FindTraceIds(traceQuery)
	if err != nil {
		return nil, err
	}

	traces := []*model.Trace{}

	for _, id := range ids {
		trace, err := s.getTrace(id)
		if err != nil {
			return nil, err
		}
		traces = append(traces, trace)
	}

	return traces, nil
}

func (s *SpanReader) getTrace(id string) (*model.Trace, error) {

	sql := fmt.Sprintf("SELECT * where TraceID = '%s' last 30 day limit 1000", id)
	resp, err := http.Get("http://localhost:9876/v1/sql?sql=" + url.QueryEscape(sql))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	data, err := simplejson.NewFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	hits := data.Get("hits").MustArray()
	spans := make([]*model.Span, 0)

	for _, _hit := range hits {
		hit := simplejson.New()
		hit.Set("data", _hit)
		values, err := hit.GetPath("data", "payload", "fields").Map()
		if err != nil {
			return nil, err
		}
		tag := make([]model.KeyValue, 0)
		process := make([]model.KeyValue, 0)
		for key, value := range values {
			if strings.HasPrefix(key, "tag.") {
				kv := model.String(strings.TrimPrefix(key, "tag."), toString(value))
				tag = append(tag, kv)
			}
			if strings.HasPrefix(key, "process.") {
				kv := model.String(strings.TrimPrefix(key, "process."), toString(value))
				process = append(process, kv)
			}
		}

		cover := dashConvert{data: values}
		TraceID, err := model.TraceIDFromString(cover.String("TraceID"))
		if err != nil {
			return nil, err
		}
		SpanID, err := model.SpanIDFromString(cover.String("SpanID"))
		if err != nil {
			return nil, err
		}

		_parentSpanID := cover.String("ParentSpanID")
		if _parentSpanID == "0" {
			_parentSpanID = ""
		}
		ParentSpanID, err := model.SpanIDFromString(_parentSpanID)

		span := &model.Span{
			TraceID:       TraceID,
			SpanID:        SpanID,
			ParentSpanID:  ParentSpanID,
			OperationName: cover.String("OperationName"),
			References:    make([]model.SpanRef, 0),
			Flags:         model.Flags(uint32(cover.Uint64("Flags"))),
			StartTime:     model.EpochMicrosecondsAsTime(cover.Uint64("StartTime") / 1000),
			Duration:      model.MicrosecondsAsDuration(cover.Uint64("Duration") / 1000),
			Tags:          tag,
			Logs:          make([]model.Log, 0),
			Process: &model.Process{
				ServiceName: cover.String("ServiceName"),
				Tags:        process,
			},
		}
		spans = append(spans, span)
	}

	trace := model.Trace{
		Spans: spans,
	}
	return &trace, nil
}

type dashConvert struct {
	data map[string]interface{}
}

func toString(v interface{}) string {
	v1 := cast.ToStringSlice(v)
	if len(v1) == 0 {
		return ""
	}
	return v1[0]
}

func (d *dashConvert) String(key string) string {
	return toString(d.data[key])
}

func (d *dashConvert) Uint64(key string) uint64 {
	v1 := cast.ToStringSlice(d.data[key])
	if len(v1) == 0 {
		return 0
	}
	return cast.ToUint64(v1[0])
}

func (d *dashConvert) Float64(key string) float64 {
	v1 := cast.ToStringSlice(d.data[key])
	if len(v1) == 0 {
		return 0
	}
	return cast.ToFloat64(v1[0])
}

func (s *SpanReader) FindTraceIds(traceQuery *spanstore.TraceQueryParameters) ([]string, error) {
	ids := []string{}
	where := "where ParentSpanID = 0 "
	if traceQuery.DurationMin.Nanoseconds() != 0 {
		where += fmt.Sprintf("and Duration > %s ", traceQuery.DurationMin.Nanoseconds())
	}
	if traceQuery.DurationMax.Nanoseconds() != 0 {
		where += fmt.Sprintf("and Duration < %s ", traceQuery.DurationMax.Nanoseconds())
	}

	sql := fmt.Sprintf("SELECT TraceID %s last 7 day limit %d", where, traceQuery.NumTraces)

	resp, err := http.Get("http://localhost:9876/v1/sql?sql=" + url.QueryEscape(sql))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	data, err := simplejson.NewFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	hits := data.Get("hits").MustArray()
	for _, _hit := range hits {
		hit := simplejson.New()
		hit.Set("data", _hit)
		values, err := hit.GetPath("data", "payload", "fields", "TraceID").Array()
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			return nil, errors.New("decode trace id failed")
		}

		ids = append(ids, values[0].(string))

	}

	return ids, nil
}
