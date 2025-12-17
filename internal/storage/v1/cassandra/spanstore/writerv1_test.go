// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
	storemocks "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestErrNewSpanWriterV1(t *testing.T) {
	session := &mocks.Session{}
	query := &mocks.Query{}
	query.On("Exec").Return(errors.New("some error"))
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, schemas[latestVersion].tableName),
		mock.Anything).Return(query)
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, schemas[previousVersion].tableName),
		mock.Anything).Return(query)
	metricsFactory := metricstest.NewFactory(0)
	logger, _ := testutils.NewLogger()
	_, err := NewSpanWriterV1(session, 0, metricsFactory, logger)
	require.ErrorContains(t, err, "neither table operation_names_v2 nor operation_names exist")
}

func TestWriteSpan(t *testing.T) {
	data, err := json.Marshal(map[string]any{"x": "y"})
	require.NoError(t, err)
	span := &model.Span{
		TraceID:       model.NewTraceID(0, 1),
		OperationName: "operation-a",
		Tags: model.KeyValues{
			model.String("x", "y"),
			model.Binary("json", data), // string tag with json value will not be inserted
		},
		Process: &model.Process{
			ServiceName: "service-a",
		},
	}
	mockWriter := &storemocks.CoreSpanWriter{}
	mockWriter.On("WriteSpan", dbmodel.FromDomain(span)).Return(nil)
	writer := SpanWriterV1{writer: mockWriter}
	require.NoError(t, writer.WriteSpan(context.Background(), span))
}

func TestSpanWriterV1Close(t *testing.T) {
	mockWriter := &storemocks.CoreSpanWriter{}
	mockWriter.On("Close").Return(nil)
	writer := SpanWriterV1{writer: mockWriter}
	require.NoError(t, writer.Close())
}
