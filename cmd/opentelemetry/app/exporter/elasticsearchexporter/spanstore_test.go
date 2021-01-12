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

package elasticsearchexporter

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter/esmodeltranslator"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/storagemetrics"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esutil"
	"github.com/jaegertracing/jaeger/pkg/cache"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
)

func TestMetrics(t *testing.T) {
	w, err := newEsSpanWriter(config.Configuration{Servers: []string{"localhost:9200"}, Version: 6}, zap.NewNop(), false, "elasticsearch")
	require.NoError(t, err)
	response := &esclient.BulkResponse{}
	response.Items = []esclient.BulkResponseItem{
		{Index: esclient.BulkIndexResponse{Status: 200}},
		{Index: esclient.BulkIndexResponse{Status: 500}},
		{Index: esclient.BulkIndexResponse{Status: 200}},
		{Index: esclient.BulkIndexResponse{Status: 500}},
	}
	blkItms := []bulkItem{
		{isService: true, spanData: esmodeltranslator.ConvertedData{
			DBSpan:                 &dbmodel.Span{},
			Span:                   pdata.NewSpan(),
			Resource:               pdata.NewResource(),
			InstrumentationLibrary: pdata.NewInstrumentationLibrary(),
		}},
		{isService: true, spanData: esmodeltranslator.ConvertedData{
			DBSpan:                 &dbmodel.Span{},
			Span:                   pdata.NewSpan(),
			Resource:               pdata.NewResource(),
			InstrumentationLibrary: pdata.NewInstrumentationLibrary(),
		}},
		{isService: false, spanData: esmodeltranslator.ConvertedData{
			DBSpan:                 &dbmodel.Span{Process: dbmodel.Process{ServiceName: "foo"}},
			Span:                   pdata.NewSpan(),
			Resource:               pdata.NewResource(),
			InstrumentationLibrary: pdata.NewInstrumentationLibrary(),
		}},
		{isService: false, spanData: esmodeltranslator.ConvertedData{
			DBSpan:                 &dbmodel.Span{Process: dbmodel.Process{ServiceName: "foo"}},
			Span:                   pdata.NewSpan(),
			Resource:               pdata.NewResource(),
			InstrumentationLibrary: pdata.NewInstrumentationLibrary(),
		}},
	}

	views := storagemetrics.MetricViews()
	require.NoError(t, view.Register(views...))
	defer view.Unregister(views...)

	failedOperations, err := w.handleResponse(context.Background(), response, blkItms)
	require.Error(t, err)
	assert.Equal(t, 1, len(failedOperations))

	viewData, err := view.RetrieveData(storagemetrics.StatSpansStoredCount().Name())
	require.NoError(t, err)
	require.Equal(t, 1, len(viewData))
	distData := viewData[0].Data.(*view.SumData)
	assert.Equal(t, float64(1), distData.Value)

	viewData, err = view.RetrieveData(storagemetrics.StatSpansNotStoredCount().Name())
	require.NoError(t, err)
	require.Equal(t, 1, len(viewData))
	distData = viewData[0].Data.(*view.SumData)
	assert.Equal(t, float64(1), distData.Value)
}

func TestBulkItemsToTraces(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		traces := bulkItemsToTraces([]bulkItem{})
		assert.Equal(t, 0, traces.SpanCount())
	})
	t.Run("one_span", func(t *testing.T) {
		span := pdata.NewSpan()
		span.SetName("name")
		resource := pdata.NewResource()
		resource.Attributes().Insert("key", pdata.NewAttributeValueString("val"))
		inst := pdata.NewInstrumentationLibrary()
		inst.SetName("name")
		traces := bulkItemsToTraces([]bulkItem{
			{
				spanData: esmodeltranslator.ConvertedData{
					Span:                   span,
					Resource:               resource,
					InstrumentationLibrary: inst,
					DBSpan:                 nil,
				},
				isService: false,
			},
		})
		expectedTraces := pdata.NewTraces()
		expectedTraces.ResourceSpans().Resize(1)
		rss := expectedTraces.ResourceSpans().At(0)
		resource.CopyTo(rss.Resource())
		rss.InstrumentationLibrarySpans().Resize(1)
		inst.CopyTo(rss.InstrumentationLibrarySpans().At(0).InstrumentationLibrary())
		rss.InstrumentationLibrarySpans().At(0).Spans().Resize(1)
		span.CopyTo(rss.InstrumentationLibrarySpans().At(0).Spans().At(0))
		assert.Equal(t, expectedTraces, traces)
	})
}

func TestWriteSpans(t *testing.T) {
	esClient := &mockESClient{
		bulkResponse: &esclient.BulkResponse{
			Errors: false,
			Items: []esclient.BulkResponseItem{
				{
					Index: esclient.BulkIndexResponse{},
				},
			},
		},
	}
	w := esSpanWriter{
		logger:           zap.NewNop(),
		client:           esClient,
		spanIndexName:    esutil.NewIndexNameProvider("span", "", "2006-01-02", esutil.AliasNone, false),
		serviceIndexName: esutil.NewIndexNameProvider("service", "", "2006-01-02", esutil.AliasNone, false),
		serviceCache:     cache.NewLRU(1),
		nameTag:          tag.Insert(storagemetrics.TagExporterName(), "name"),
	}

	t.Run("zero_spans_failed", func(t *testing.T) {
		dropped, err := w.writeSpans(context.Background(), []esmodeltranslator.ConvertedData{
			{
				DBSpan: &dbmodel.Span{},
			},
		})
		assert.Equal(t, 0, dropped)
		assert.NoError(t, err)
		esClient.bulkResponse = &esclient.BulkResponse{
			Items: []esclient.BulkResponseItem{
				{
					Index: esclient.BulkIndexResponse{
						Status: 500,
					},
				},
			},
		}
	})
	t.Run("one_span_failed", func(t *testing.T) {
		span := pdata.NewSpan()
		span.SetName("name")
		resource := pdata.NewResource()
		resource.Attributes().Insert("key", pdata.NewAttributeValueString("val"))
		inst := pdata.NewInstrumentationLibrary()
		inst.SetName("name")
		traces := bulkItemsToTraces([]bulkItem{{
			spanData: esmodeltranslator.ConvertedData{
				Span:                   span,
				Resource:               resource,
				InstrumentationLibrary: inst,
				DBSpan:                 nil,
			},
			isService: false,
		}})

		dropped, err := w.writeSpans(context.Background(), []esmodeltranslator.ConvertedData{
			{
				DBSpan:                 &dbmodel.Span{},
				Span:                   span,
				Resource:               resource,
				InstrumentationLibrary: inst,
			},
		})
		assert.Equal(t, 1, dropped)
		assert.Error(t, err)
		partialErr, ok := err.(consumererror.PartialError)
		require.True(t, ok)
		assert.Equal(t, traces, partialErr.GetTraces())
	})
}

type mockESClient struct {
	bulkResponse *esclient.BulkResponse
}

var _ esclient.ElasticsearchClient = (*mockESClient)(nil)

func (m mockESClient) PutTemplate(ctx context.Context, name string, template io.Reader) error {
	panic("implement me")
}

func (m mockESClient) Bulk(ctx context.Context, bulkBody io.Reader) (*esclient.BulkResponse, error) {
	return m.bulkResponse, nil
}

func (m mockESClient) AddDataToBulkBuffer(bulkBody *bytes.Buffer, data []byte, index, typ string) {
}

func (m mockESClient) Index(ctx context.Context, body io.Reader, index, typ string) error {
	panic("implement me")
}

func (m mockESClient) Search(ctx context.Context, query esclient.SearchBody, size int, indices ...string) (*esclient.SearchResponse, error) {
	panic("implement me")
}

func (m mockESClient) MultiSearch(ctx context.Context, queries []esclient.SearchBody) (*esclient.MultiSearchResponse, error) {
	panic("implement me")
}

func (m mockESClient) MajorVersion() int {
	panic("implement me")
}
