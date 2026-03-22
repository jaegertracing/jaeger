// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
)

func TestNewTraceWriter(t *testing.T) {
	writer := NewTraceWriter(&mocks.CoreSpanWriter{})
	require.NotNil(t, writer)
}

func TestTraceWriter_WriteTraces(t *testing.T) {
	_, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))
	td := FromDBModel([]dbmodel.Span{span})

	mockWriter := &mocks.CoreSpanWriter{}
	mockWriter.On("WriteDbSpan", mock.Anything, mock.Anything).Return(nil)

	w := NewTraceWriter(mockWriter)
	err := w.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	mockWriter.AssertNumberOfCalls(t, "WriteDbSpan", 1)
	mockWriter.AssertExpectations(t)
}

func TestTraceWriter_WriteTraces_Error(t *testing.T) {
	_, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))
	td := FromDBModel([]dbmodel.Span{span})

	mockWriter := mocks.NewCoreSpanWriter(t)
	mockWriter.On("WriteDbSpan", mock.Anything, mock.Anything).Return(errors.New("write error"))

	w := NewTraceWriter(mockWriter)
	err := w.WriteTraces(context.Background(), td)
	require.ErrorContains(t, err, "write error")
	mockWriter.AssertExpectations(t)
}

func TestTraceWriter_WriteTraces_MultiSpanMixedErrors(t *testing.T) {
	_, spanStr := loadFixtures(t, 1)
	var span1, span2, span3 dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span1))
	require.NoError(t, json.Unmarshal(spanStr, &span2))
	require.NoError(t, json.Unmarshal(spanStr, &span3))
	td := FromDBModel([]dbmodel.Span{span1, span2, span3})

	mockWriter := &mocks.CoreSpanWriter{}
	err1 := errors.New("write error 1")
	err2 := errors.New("write error 2")
	mockWriter.On("WriteDbSpan", mock.Anything, mock.Anything).Return(nil).Once()
	mockWriter.On("WriteDbSpan", mock.Anything, mock.Anything).Return(err1).Once()
	mockWriter.On("WriteDbSpan", mock.Anything, mock.Anything).Return(err2).Once()

	w := NewTraceWriter(mockWriter)
	err := w.WriteTraces(context.Background(), td)
	require.Error(t, err)
	require.ErrorContains(t, err, "write error 1")
	require.ErrorContains(t, err, "write error 2")
	mockWriter.AssertNumberOfCalls(t, "WriteDbSpan", 3)
	mockWriter.AssertExpectations(t)
}

func TestTraceWriter_WriteTraces_ContextCancelled(t *testing.T) {
	_, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))

	td := FromDBModel([]dbmodel.Span{span, span, span})
	mockWriter := &mocks.CoreSpanWriter{}
	w := NewTraceWriter(mockWriter)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := w.WriteTraces(ctx, td)

	require.ErrorIs(t, err, context.Canceled)
	mockWriter.AssertNotCalled(t, "WriteDbSpan")
}
