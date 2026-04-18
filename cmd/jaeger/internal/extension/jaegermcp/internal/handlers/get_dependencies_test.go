// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

type mockDependencyService struct {
	deps []model.DependencyLink
	err  error
}

func (m *mockDependencyService) GetDependencies(_ context.Context, _ time.Time, _ time.Duration) ([]model.DependencyLink, error) {
	return m.deps, m.err
}

func TestGetDependenciesHandler_Handle_Success(t *testing.T) {
	deps := []model.DependencyLink{
		{Parent: "api-gateway", Child: "payment-service", CallCount: 150},
		{Parent: "payment-service", Child: "database", CallCount: 300},
		{Parent: "api-gateway", Child: "user-service", CallCount: 200},
	}
	h := &getDependenciesHandler{queryService: &mockDependencyService{deps: deps}}

	_, output, err := h.handle(context.Background(), nil, types.GetDependenciesInput{})
	require.NoError(t, err)
	require.Len(t, output.Dependencies, 3)
	assert.Equal(t, "api-gateway", output.Dependencies[0].Parent)
	assert.Equal(t, "payment-service", output.Dependencies[0].Child)
	assert.Equal(t, uint64(150), output.Dependencies[0].CallCount)
}

func TestGetDependenciesHandler_Handle_WithLookback(t *testing.T) {
	h := &getDependenciesHandler{queryService: &mockDependencyService{}}

	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{Lookback: "1h"})
	require.NoError(t, err)
}

func TestGetDependenciesHandler_Handle_InvalidLookback(t *testing.T) {
	h := &getDependenciesHandler{queryService: &mockDependencyService{}}

	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{Lookback: "invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid lookback")
}

func TestGetDependenciesHandler_Handle_StorageError(t *testing.T) {
	h := &getDependenciesHandler{
		queryService: &mockDependencyService{err: errors.New("storage unavailable")},
	}

	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get dependencies")
}

func TestGetDependenciesHandler_Handle_EmptyResult(t *testing.T) {
	h := &getDependenciesHandler{queryService: &mockDependencyService{}}

	_, output, err := h.handle(context.Background(), nil, types.GetDependenciesInput{})
	require.NoError(t, err)
	assert.Empty(t, output.Dependencies)
}
