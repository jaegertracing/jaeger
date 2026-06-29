// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/mocks"
)

// stubSummaryReader embeds the generated core.Reader mock (to satisfy core.Reader)
// and adds native summary support, so it also satisfies summaryAggregator.
type stubSummaryReader struct {
	*mocks.Reader
	summaries []dbmodel.TraceSummary
	err       error
}

func (s stubSummaryReader) FindTraceSummaries(context.Context, dbmodel.TraceQueryParameters) ([]dbmodel.TraceSummary, error) {
	return s.summaries, s.err
}

func collectSummaries(seq iter.Seq2[[]tracestore.TraceSummary, error]) ([]tracestore.TraceSummary, error) {
	var out []tracestore.TraceSummary
	for batch, err := range seq {
		if err != nil {
			return nil, err
		}
		out = append(out, batch...)
	}
	return out, nil
}

func emptyQuery() tracestore.TraceQueryParams {
	return tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}
}

func TestTraceReader_FindTraceSummaries(t *testing.T) {
	dbSummaries := []dbmodel.TraceSummary{{
		TraceID:           "00000000000000000000000000000001",
		RootServiceName:   "svcA",
		RootOperationName: "root-op",
		MinStartTime:      1000000,
		MaxEndTime:        2000000,
		SpanCount:         3,
		ErrorSpanCount:    1,
		Services: []dbmodel.ServiceSummary{
			{ServiceName: "svcA", SpanCount: 2, ErrorSpanCount: 1},
			{ServiceName: "svcB", SpanCount: 1},
		},
	}}
	reader := ReaderWithSummaries{TraceReader: TraceReader{spanReader: stubSummaryReader{Reader: &mocks.Reader{}, summaries: dbSummaries}}}

	got, err := collectSummaries(reader.FindTraceSummaries(context.Background(), emptyQuery()))
	require.NoError(t, err)
	require.Len(t, got, 1)

	s := got[0]
	expectedID, idErr := convertTraceIDFromDB("00000000000000000000000000000001")
	require.NoError(t, idErr)
	assert.Equal(t, expectedID, s.TraceID)
	assert.Equal(t, "svcA", s.RootServiceName)
	assert.Equal(t, "root-op", s.RootOperationName)
	assert.Equal(t, 3, s.SpanCount)
	assert.Equal(t, 1, s.ErrorSpanCount)
	assert.Equal(t, 0, s.OrphanSpanCount)
	assert.Equal(t, time.UnixMicro(1000000).UTC(), s.MinStartTime)
	assert.Equal(t, time.UnixMicro(2000000).UTC(), s.MaxEndTime)

	require.Len(t, s.Services, 2)
	assert.Equal(t, "svcA", s.Services[0].Name)
	assert.Equal(t, 2, s.Services[0].SpanCount)
	assert.Equal(t, 1, s.Services[0].ErrorSpanCount)
	assert.Equal(t, "svcB", s.Services[1].Name)
}

func TestTraceReader_FindTraceSummaries_AggregatorError(t *testing.T) {
	reader := ReaderWithSummaries{TraceReader: TraceReader{spanReader: stubSummaryReader{Reader: &mocks.Reader{}, err: errors.New("boom")}}}
	_, err := collectSummaries(reader.FindTraceSummaries(context.Background(), emptyQuery()))
	require.Error(t, err)
}

func TestTraceReader_FindTraceSummaries_Unsupported(t *testing.T) {
	// A plain core.Reader mock does not implement summaryAggregator.
	reader := ReaderWithSummaries{TraceReader: TraceReader{spanReader: &mocks.Reader{}}}
	_, err := collectSummaries(reader.FindTraceSummaries(context.Background(), emptyQuery()))
	require.ErrorIs(t, err, errors.ErrUnsupported)
}

func TestTraceReader_FindTraceSummaries_BadTraceID(t *testing.T) {
	dbSummaries := []dbmodel.TraceSummary{{TraceID: "not-hex"}}
	reader := ReaderWithSummaries{TraceReader: TraceReader{spanReader: stubSummaryReader{Reader: &mocks.Reader{}, summaries: dbSummaries}}}
	_, err := collectSummaries(reader.FindTraceSummaries(context.Background(), emptyQuery()))
	require.Error(t, err)
}
