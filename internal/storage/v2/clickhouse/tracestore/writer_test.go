// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestWriteTraces(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockClient := mocks.NewConn(t)
		writer := Writer{
			client: mockClient,
		}
		mockBatch := mocks.NewBatch(t)
		mockBatch.On("Append").Return(nil)
		mockBatch.On("Send").Return(nil)
		mockClient.On("PrepareBatch", mock.Anything, mock.Anything).Return(mockBatch, nil)

		err := writer.WriteTraces(context.Background(), ptrace.NewTraces())
		require.NoError(t, err)
	})
	t.Run("failed when call prepareBatch not works", func(t *testing.T) {
		mockClient := mocks.NewConn(t)
		writer := Writer{
			client: mockClient,
		}
		mockClient.On("PrepareBatch", mock.Anything, mock.Anything).Return(nil, errors.New("connect to server timeout"))

		err := writer.WriteTraces(context.Background(), ptrace.NewTraces())
		require.ErrorContains(t, err, "connect to server timeout")
	})
	t.Run("failed when call Append not works", func(t *testing.T) {
		mockClient := mocks.NewConn(t)
		writer := Writer{
			client: mockClient,
		}
		mockBatch := mocks.NewBatch(t)
		mockClient.On("PrepareBatch", mock.Anything, mock.Anything).
			Return(mockBatch, nil)
		mockBatch.On("Append").Return(errors.New("connect to server timeout"))

		err := writer.WriteTraces(context.Background(), ptrace.NewTraces())
		require.ErrorContains(t, err, "connect to server timeout")
	})
	t.Run("failed when call Send not works", func(t *testing.T) {
		mockClient := mocks.NewConn(t)
		writer := Writer{
			client: mockClient,
		}
		mockBatch := mocks.NewBatch(t)
		mockClient.On("PrepareBatch", mock.Anything, mock.Anything).Return(mockBatch, nil)
		mockBatch.On("Append").Return(nil)
		mockBatch.On("Send").Return(errors.New("connect to server timeout"))

		err := writer.WriteTraces(context.Background(), ptrace.NewTraces())
		require.ErrorContains(t, err, "connect to server timeout")
	})
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
