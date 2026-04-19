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
	deps             []model.DependencyLink
	err              error
	capturedEndTs    time.Time
	capturedLookback time.Duration
}

func (m *mockDependencyService) GetDependencies(_ context.Context, endTs time.Time, lookback time.Duration) ([]model.DependencyLink, error) {
	m.capturedEndTs = endTs
	m.capturedLookback = lookback
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
	assert.Equal(t, "api-gateway", output.Dependencies[0].Caller)
	assert.Equal(t, "payment-service", output.Dependencies[0].Callee)
	assert.Equal(t, uint64(150), output.Dependencies[0].CallCount)
}

func TestGetDependenciesHandler_Handle_DefaultTimeRange(t *testing.T) {
	mock := &mockDependencyService{}
	h := &getDependenciesHandler{queryService: mock}

	before := time.Now()
	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{})
	require.NoError(t, err)

	// Default lookback should be 24h
	assert.InDelta(t, (24 * time.Hour).Seconds(), mock.capturedLookback.Seconds(), 1)
	// End time should be approximately now
	assert.WithinDuration(t, before, mock.capturedEndTs, 2*time.Second)
}

func TestGetDependenciesHandler_Handle_CustomTimeRange(t *testing.T) {
	mock := &mockDependencyService{}
	h := &getDependenciesHandler{queryService: mock}

	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{
		StartTime: "-1h",
		EndTime:   "now",
	})
	require.NoError(t, err)
	assert.InDelta(t, time.Hour.Seconds(), mock.capturedLookback.Seconds(), 1)
}

func TestGetDependenciesHandler_Handle_RFC3339TimeRange(t *testing.T) {
	mock := &mockDependencyService{}
	h := &getDependenciesHandler{queryService: mock}

	endTime := time.Now().UTC().Truncate(time.Second)
	startTime := endTime.Add(-2 * time.Hour)

	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{
		StartTime: startTime.Format(time.RFC3339),
		EndTime:   endTime.Format(time.RFC3339),
	})
	require.NoError(t, err)
	assert.InDelta(t, (2 * time.Hour).Seconds(), mock.capturedLookback.Seconds(), 1)
	assert.WithinDuration(t, endTime, mock.capturedEndTs, time.Second)
}

func TestGetDependenciesHandler_Handle_InvalidStartTime(t *testing.T) {
	h := &getDependenciesHandler{queryService: &mockDependencyService{}}

	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{StartTime: "invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid start_time")
}

func TestGetDependenciesHandler_Handle_InvalidEndTime(t *testing.T) {
	h := &getDependenciesHandler{queryService: &mockDependencyService{}}

	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{EndTime: "invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid end_time")
}

func TestGetDependenciesHandler_Handle_StartAfterEnd(t *testing.T) {
	h := &getDependenciesHandler{queryService: &mockDependencyService{}}

	_, _, err := h.handle(context.Background(), nil, types.GetDependenciesInput{
		StartTime: "now",
		EndTime:   "-1h",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start_time must be before end_time")
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
