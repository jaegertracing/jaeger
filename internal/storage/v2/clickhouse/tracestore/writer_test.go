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

const connectErrorMsg = "connect to server timeout"

func TestWriteTraces_Success(t *testing.T) {
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
}

func TestWriteTraces_Error(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*mocks.Conn, *mocks.Batch)
		expectedError string
	}{
		{
			name: "call prepareBatch failed",
			setupMocks: func(mockClient *mocks.Conn, mockBatch *mocks.Batch) {
				mockClient.On("PrepareBatch", mock.Anything, mock.Anything).Return(nil, errors.New(connectErrorMsg))
				mockBatch.AssertNotCalled(t, "Append")
			},
			expectedError: connectErrorMsg,
		},
		{
			name: "call Append failed",
			setupMocks: func(mockClient *mocks.Conn, mockBatch *mocks.Batch) {
				mockClient.On("PrepareBatch", mock.Anything, mock.Anything).Return(mockBatch, nil)
				mockBatch.On("Append").Return(errors.New(connectErrorMsg))
			},
			expectedError: connectErrorMsg,
		},
		{
			name: "call Send failed",
			setupMocks: func(mockClient *mocks.Conn, mockBatch *mocks.Batch) {
				mockClient.On("PrepareBatch", mock.Anything, mock.Anything).Return(mockBatch, nil)
				mockBatch.On("Append").Return(nil)
				mockBatch.On("Send").Return(errors.New(connectErrorMsg))
			},
			expectedError: connectErrorMsg,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := mocks.NewConn(t)
			mockBatch := mocks.NewBatch(t)

			writer := Writer{
				client: mockClient,
			}

			tt.setupMocks(mockClient, mockBatch)
			err := writer.WriteTraces(context.Background(), ptrace.NewTraces())
			require.ErrorContains(t, err, tt.expectedError)
		})
	}
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
