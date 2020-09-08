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
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opencensus.io/stats/view"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/storagemetrics"
	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/internal/esclient"
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
		{isService: true, span: &dbmodel.Span{}},
		{isService: true, span: &dbmodel.Span{}},
		{span: &dbmodel.Span{Process: dbmodel.Process{ServiceName: "foo"}}},
		{span: &dbmodel.Span{Process: dbmodel.Process{ServiceName: "foo"}}},
	}

	views := storagemetrics.MetricViews()
	require.NoError(t, view.Register(views...))
	defer view.Unregister(views...)

	errs := w.handleResponse(context.Background(), response, blkItms)
	assert.Equal(t, 2, errs)

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
