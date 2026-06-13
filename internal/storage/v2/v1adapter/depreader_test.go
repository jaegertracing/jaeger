// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	dependencystoremocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
)

func TestGetV1DependencyReader(t *testing.T) {
	t.Run("wrapped v1 reader", func(t *testing.T) {
		reader := new(dependencystoremocks.Reader)
		traceReader := &DependencyReader{
			reader: reader,
		}
		v1Reader := GetV1DependencyReader(traceReader)
		require.Equal(t, reader, v1Reader)
	})

	t.Run("native v2 reader", func(t *testing.T) {
		reader := new(depstoremocks.Reader)
		v1Reader := GetV1DependencyReader(reader)
		require.IsType(t, &DowngradedDependencyReader{}, v1Reader)
		require.Equal(t, reader, v1Reader.(*DowngradedDependencyReader).reader)
	})
}

func TestDependencyReader_GetDependencies(t *testing.T) {
	end := time.Now()
	start := end.Add(-1 * time.Minute)
	query := depstore.QueryParameters{
		StartTime: start,
		EndTime:   end,
	}
	expectedDeps := []model.DependencyLink{{Parent: "parent", Child: "child", CallCount: 12}}
	mr := new(dependencystoremocks.Reader)
	mr.On("GetDependencies", mock.Anything, end, time.Minute).Return(expectedDeps, nil)
	dr := NewDependencyReader(mr)
	deps, err := dr.GetDependencies(context.Background(), query)
	require.NoError(t, err)
	require.Equal(t, expectedDeps, deps)
}

func TestDowngradedDependencyReader_GetDependencies(t *testing.T) {
	end := time.Now()
	start := end.Add(-1 * time.Minute)
	query := depstore.QueryParameters{
		StartTime: start,
		EndTime:   end,
	}
	expectedDeps := []model.DependencyLink{{Parent: "parent", Child: "child", CallCount: 12}}
	mr := new(depstoremocks.Reader)
	mr.On("GetDependencies", mock.Anything, query).Return(expectedDeps, nil)
	dr := &DowngradedDependencyReader{
		reader: mr,
	}
	deps, err := dr.GetDependencies(context.Background(), end, time.Minute)
	require.NoError(t, err)
	require.Equal(t, expectedDeps, deps)
}
