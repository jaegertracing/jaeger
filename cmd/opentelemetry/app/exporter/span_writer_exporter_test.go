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

package exporter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/storagemetrics"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestNew_closableWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, component.ExporterCreateParams{Logger: zap.NewNop()}, mockStorageFactory{spanWriter: spanWriter{}})
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.Nil(t, exporter.Shutdown(context.Background()))
}

func TestNew_noClosableWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, component.ExporterCreateParams{Logger: zap.NewNop()}, mockStorageFactory{spanWriter: noClosableWriter{}})
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.Nil(t, exporter.Shutdown(context.Background()))
}

func TestNew_failedToCreateWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, component.ExporterCreateParams{Logger: zap.NewNop()}, mockStorageFactory{err: errors.New("failed to create writer"), spanWriter: spanWriter{}})
	require.Nil(t, exporter)
	assert.Error(t, err, "failed to create writer")
}

func traces() pdata.Traces {
	traces := pdata.NewTraces()
	traces.ResourceSpans().Resize(1)
	traces.ResourceSpans().At(0).InstrumentationLibrarySpans().Resize(1)
	return traces
}

func AddSpan(traces pdata.Traces, name string, traceID pdata.TraceID, spanID pdata.SpanID) pdata.Traces {
	rspans := traces.ResourceSpans()
	instSpans := rspans.At(0).InstrumentationLibrarySpans()
	spans := instSpans.At(0).Spans()
	spans.Resize(spans.Len() + 1)
	span := spans.At(spans.Len() - 1)
	span.SetName(name)
	span.SetTraceID(traceID)
	span.SetSpanID(spanID)
	return traces
}

func TestStore(t *testing.T) {
	traceID := pdata.NewTraceID([16]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
		0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F})
	spanID := pdata.NewSpanID([8]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	tests := []struct {
		storage         store
		data            pdata.Traces
		err             string
		dropped         int
		caption         string
		metricStored    float64
		metricNotStored float64
	}{
		{
			caption: "nothing to store",
			storage: store{Writer: spanWriter{}, storageNameTag: tag.Insert(storagemetrics.TagExporterName(), "memory")},
			data:    traces(),
			dropped: 0,
		},
		{
			caption: "wrong data",
			storage: store{Writer: spanWriter{}, storageNameTag: tag.Insert(storagemetrics.TagExporterName(), "memory")},
			data:    AddSpan(traces(), "", pdata.NewTraceID([16]byte{}), pdata.NewSpanID([8]byte{})),
			err:     "Permanent error: OC span has an all zeros trace ID",
			dropped: 1,
		},
		{
			caption:         "one error in writer",
			storage:         store{Writer: spanWriter{err: errors.New("could not store")}, storageNameTag: tag.Insert(storagemetrics.TagExporterName(), "memory")},
			data:            AddSpan(AddSpan(traces(), "error", traceID, spanID), "", traceID, spanID),
			dropped:         1,
			err:             "could not store",
			metricNotStored: 1,
			metricStored:    1,
		},
		{
			caption:         "two errors in writer",
			storage:         store{Writer: spanWriter{err: errors.New("could not store")}, storageNameTag: tag.Insert(storagemetrics.TagExporterName(), "memory")},
			data:            AddSpan(AddSpan(traces(), "error", traceID, spanID), "error", traceID, spanID),
			dropped:         2,
			err:             "[could not store; could not store]",
			metricNotStored: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.caption, func(t *testing.T) {
			views := storagemetrics.MetricViews()
			require.NoError(t, view.Register(views...))
			defer view.Unregister(views...)

			dropped, err := test.storage.traceDataPusher(context.Background(), test.data)
			assert.Equal(t, test.dropped, dropped)
			if test.err != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.err)
			} else {
				require.NoError(t, err)
			}

			if test.metricStored > 0 {
				viewData, err := view.RetrieveData(storagemetrics.StatSpansStoredCount().Name())
				require.NoError(t, err)
				require.Equal(t, 1, len(viewData))
				distData := viewData[0].Data.(*view.SumData)
				assert.Equal(t, test.metricStored, distData.Value)
			}
			if test.metricNotStored > 0 {
				viewData, err := view.RetrieveData(storagemetrics.StatSpansNotStoredCount().Name())
				require.NoError(t, err)
				require.Equal(t, 1, len(viewData))
				distData := viewData[0].Data.(*view.SumData)
				assert.Equal(t, test.metricNotStored, distData.Value)
			}
		})
	}
}

type spanWriter struct {
	err error
}

func (w spanWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	if span.GetOperationName() == "error" {
		return w.err
	}
	return nil
}

func (spanWriter) Close() error {
	return nil
}

type noClosableWriter struct {
}

func (noClosableWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	return nil
}

type mockStorageFactory struct {
	err        error
	spanWriter spanstore.Writer
}

func (m mockStorageFactory) CreateSpanWriter() (spanstore.Writer, error) {
	return m.spanWriter, m.err
}
func (mockStorageFactory) CreateSpanReader() (spanstore.Reader, error) {
	return nil, nil
}
func (mockStorageFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	return nil, nil
}
func (mockStorageFactory) Initialize(metrics.Factory, *zap.Logger) error {
	return nil
}
