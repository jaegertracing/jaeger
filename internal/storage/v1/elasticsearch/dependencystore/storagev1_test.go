// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencystore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore/mocks"
)

func TestV1WriteDependencies(t *testing.T) {
	coreDependencyStore := &mocks.CoreDependencyStore{}
	depStore := StoreV1{depStore: coreDependencyStore}
	dependencies := []model.DependencyLink{
		{
			Parent:    "hello",
			Child:     "world",
			CallCount: 12,
		},
	}
	dbDependencies := dbmodel.FromDomainDependencies(dependencies)
	ts := time.Now()
	coreDependencyStore.On("WriteDependencies", ts, dbDependencies).Return(nil)
	err := depStore.WriteDependencies(ts, dependencies)
	require.NoError(t, err)
}

func TestV1CreateTemplates(t *testing.T) {
	coreDependencyStore := &mocks.CoreDependencyStore{}
	depStore := StoreV1{depStore: coreDependencyStore}
	templateName := "testing-template"
	coreDependencyStore.On("CreateTemplates", templateName).Return(nil)
	err := depStore.CreateTemplates(templateName)
	require.NoError(t, err)
}

func TestV1GetDependencies(t *testing.T) {
	tests := []struct {
		name                  string
		returningDependencies []dbmodel.DependencyLink
		returningErr          error
		expectedDependencies  []model.DependencyLink
		expectedErr           string
	}{
		{
			name: "no error",
			returningDependencies: []dbmodel.DependencyLink{
				{
					Parent:    "hello",
					Child:     "world",
					CallCount: 12,
				},
			},
			expectedDependencies: []model.DependencyLink{
				{
					Parent:    "hello",
					Child:     "world",
					CallCount: 12,
				},
			},
		},
		{
			name:         "error",
			returningErr: errors.New("some error"),
			expectedErr:  "some error",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coreDependencyStore := &mocks.CoreDependencyStore{}
			depStore := StoreV1{depStore: coreDependencyStore}
			ts := time.Now()
			drn := 24 * time.Hour
			coreDependencyStore.On("GetDependencies", mock.Anything, ts, drn).Return(test.returningDependencies, test.returningErr)
			deps, err := depStore.GetDependencies(context.Background(), ts, drn)
			if test.expectedErr != "" {
				require.ErrorContains(t, err, test.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedDependencies, deps)
			}
		})
	}
}
