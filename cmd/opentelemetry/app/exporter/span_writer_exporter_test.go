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
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestNew_closableWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, mockStorageFactory{spanWriter: spanWriter{}})
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.Nil(t, exporter.Shutdown(context.Background()))
}

func TestNew_noClosableWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, mockStorageFactory{spanWriter: noClosableWriter{}})
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.Nil(t, exporter.Shutdown(context.Background()))
}

func TestNew_failedToCreateWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, mockStorageFactory{err: errors.New("failed to create writer"), spanWriter: spanWriter{}})
	require.Nil(t, exporter)
	assert.Error(t, err, "failed to create writer")
}

func traces() pdata.Traces {
	traces := pdata.NewTraces()
	traces.ResourceSpans().Resize(1)
	traces.ResourceSpans().At(0).InstrumentationLibrarySpans().Resize(1)
	return traces
}

func AddSpan(traces pdata.Traces, name string, traceID []byte, spanID []byte) pdata.Traces {
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
	traceID := []byte("0123456789abcdef")
	spanID := []byte("01234567")
	tests := []struct {
		storage store
		data    pdata.Traces
		err     string
		dropped int
		caption string
	}{
		{
			caption: "nothing to store",
			storage: store{Writer: spanWriter{}},
			data:    traces(),
			dropped: 0,
		},
		{
			caption: "wrong data",
			storage: store{Writer: spanWriter{}},
			data:    AddSpan(traces(), "", nil, nil),
			err:     "TraceID is nil",
			dropped: 1,
		},
		{
			caption: "one error in writer",
			storage: store{Writer: spanWriter{err: errors.New("could not store")}},
			data:    AddSpan(AddSpan(traces(), "error", traceID, spanID), "", traceID, spanID),
			dropped: 1,
			err:     "could not store",
		},
		{
			caption: "two errors in writer",
			storage: store{Writer: spanWriter{err: errors.New("could not store")}},
			data:    AddSpan(AddSpan(traces(), "error", traceID, spanID), "error", traceID, spanID),
			dropped: 2,
			err:     "[could not store; could not store]",
		},
	}
	for _, test := range tests {
		t.Run(test.caption, func(t *testing.T) {
			dropped, err := test.storage.traceDataPusher(context.Background(), test.data)
			assert.Equal(t, test.dropped, dropped)
			if test.err != "" {
				assert.Contains(t, err.Error(), test.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type spanWriter struct {
	err error
}

func (w spanWriter) WriteSpan(span *model.Span) error {
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

func (noClosableWriter) WriteSpan(span *model.Span) error {
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
