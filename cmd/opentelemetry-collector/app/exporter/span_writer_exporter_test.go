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

	tracepb "github.com/census-instrumentation/opencensus-proto/gen-go/trace/v1"
	"github.com/open-telemetry/opentelemetry-collector/config/configmodels"
	"github.com/open-telemetry/opentelemetry-collector/consumer/consumerdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestNew(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, spanWriter{})
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.Nil(t, exporter.Shutdown())
	exporter, err = NewSpanWriterExporter(&configmodels.ExporterSettings{}, noClosableWriter{})
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.Nil(t, exporter.Shutdown())
}

func TestStore(t *testing.T) {
	traceID := []byte("0123456789abcdef")
	spanId := []byte("01234567")
	errorName := &tracepb.TruncatableString{Value: "error"}
	tests := []struct {
		storage storage
		data    consumerdata.TraceData
		err     string
		dropped int
		caption string
	}{
		{
			caption: "nothing to store",
			storage: storage{Writer: spanWriter{}},
			data:    consumerdata.TraceData{Spans: []*tracepb.Span{}},
			dropped: 0,
		},
		{
			caption: "wrong data",
			storage: storage{Writer: spanWriter{}},
			data:    consumerdata.TraceData{Spans: []*tracepb.Span{{}}},
			err:     "TraceID is nil",
			dropped: 1,
		},
		{
			caption: "one error in writer",
			storage: storage{Writer: spanWriter{err: errors.New("could not store")}},
			data: consumerdata.TraceData{Spans: []*tracepb.Span{
				{TraceId: traceID, SpanId: spanId, Name: errorName},
				{TraceId: traceID, SpanId: spanId},
			}},
			dropped: 1,
			err:     "could not store",
		},
		{
			caption: "two errors in writer",
			storage: storage{Writer: spanWriter{err: errors.New("could not store")}},
			data: consumerdata.TraceData{Spans: []*tracepb.Span{
				{TraceId: traceID, SpanId: spanId, Name: errorName},
				{TraceId: traceID, SpanId: spanId, Name: errorName},
			}},
			dropped: 2,
			err:     "[could not store; could not store]",
		},
	}
	for _, test := range tests {
		t.Run(test.caption, func(t *testing.T) {
			dropped, err := test.storage.traceDataPusher(context.Background(), test.data)
			assert.Equal(t, test.dropped, dropped)
			if test.err != "" {
				assert.EqualError(t, err, test.err)
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
