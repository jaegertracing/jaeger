// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/mocks"
)

func TestV2GetDependencies(t *testing.T) {
	tests := []struct {
		name           string
		mockOutput     []dbmodel.DependencyLink
		mockErr        error
		expectedOutput []model.DependencyLink
		expectedErr    string
	}{
		{
			name:        "error from core reader",
			mockErr:     errors.New("error from core reader"),
			expectedErr: "error from core reader",
		},
		{
			name: "success output",
			mockOutput: []dbmodel.DependencyLink{
				{
					Parent:    "testing-parent",
					Child:     "testing-child",
					CallCount: 1,
				},
			},
			expectedOutput: []model.DependencyLink{
				{
					Parent:    "testing-parent",
					Child:     "testing-child",
					CallCount: 1,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreReader := &mocks.CoreDependencyStore{}
			store := DependencyStoreV2{
				store: coreReader,
			}
			query := depstore.QueryParameters{
				StartTime: time.Now(),
				EndTime:   time.Now(),
			}
			coreReader.On("GetDependencies", mock.Anything, query.EndTime, query.EndTime.Sub(query.StartTime)).Return(tt.mockOutput, tt.mockErr)
			actual, err := store.GetDependencies(context.Background(), query)
			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedOutput, actual)
			}
		})
	}
}

func TestV2WriteDependencies(t *testing.T) {
	tests := []struct {
		name         string
		returningErr error
		expectedErr  string
	}{
		{
			name:         "error from core writer",
			returningErr: errors.New("error from core writer"),
			expectedErr:  "error from core writer",
		},
		{
			name: "success",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreReader := &mocks.CoreDependencyStore{}
			store := DependencyStoreV2{
				store: coreReader,
			}
			coreReader.On("WriteDependencies", mock.Anything, mock.Anything).Return(tt.returningErr)
			err := store.WriteDependencies(time.Now(), []model.DependencyLink{})
			if tt.expectedErr != "" {
				assert.ErrorContains(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewDependencyStoreV2(t *testing.T) {
	store := NewDependencyStoreV2(Params{Logger: zap.NewNop()})
	assert.IsType(t, &DependencyStore{}, store.store)
}
