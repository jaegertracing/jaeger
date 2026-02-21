// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegermcp/internal/types"
)

// mockGetServicesQueryService mocks the GetServices method for testing
type mockGetServicesQueryService struct {
	getServicesFunc func(ctx context.Context) ([]string, error)
}

func (m *mockGetServicesQueryService) GetServices(ctx context.Context) ([]string, error) {
	if m.getServicesFunc != nil {
		return m.getServicesFunc(ctx)
	}
	return nil, nil
}

func TestGetServicesHandler_Success_AllServices(t *testing.T) {
	testServices := []string{
		"frontend",
		"payment-service",
		"cart-service",
		"user-service",
	}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	input := types.GetServicesInput{}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Equal(t, testServices, output.Services)
}

func TestGetServicesHandler_Success_WithPattern(t *testing.T) {
	testServices := []string{
		"frontend",
		"payment-service",
		"payment-gateway",
		"cart-service",
		"user-service",
	}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	// Filter for services containing "payment"
	input := types.GetServicesInput{
		Pattern: "payment",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Services, 2)
	assert.Contains(t, output.Services, "payment-service")
	assert.Contains(t, output.Services, "payment-gateway")
}

func TestGetServicesHandler_Success_WithRegexPattern(t *testing.T) {
	testServices := []string{
		"frontend-prod",
		"frontend-staging",
		"payment-service",
		"cart-service-prod",
		"cart-service-staging",
	}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	// Filter for services ending with "-prod"
	input := types.GetServicesInput{
		Pattern: "-prod$",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Services, 2)
	assert.Contains(t, output.Services, "frontend-prod")
	assert.Contains(t, output.Services, "cart-service-prod")
}

func TestGetServicesHandler_Success_WithLimit(t *testing.T) {
	testServices := []string{
		"service-5",
		"service-3",
		"service-1",
		"service-4",
		"service-2",
	}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	input := types.GetServicesInput{
		Limit: 3,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Services, 3)
	// After sorting, should return first 3 alphabetically
	assert.Equal(t, []string{"service-1", "service-2", "service-3"}, output.Services)
}

func TestGetServicesHandler_Success_WithPatternAndLimit(t *testing.T) {
	testServices := []string{
		"payment-staging",
		"frontend-prod",
		"cart-staging",
		"frontend-staging",
		"payment-prod",
		"cart-prod",
	}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	// Filter for services ending with "-prod" and limit to 2
	input := types.GetServicesInput{
		Pattern: "-prod$",
		Limit:   2,
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Services, 2)
	// After filtering and sorting, should return first 2 alphabetically
	assert.Equal(t, []string{"cart-prod", "frontend-prod"}, output.Services)
}

func TestGetServicesHandler_Success_DefaultLimit(t *testing.T) {
	// Create more than default limit services
	testServices := make([]string, 150)
	for i := range 150 {
		testServices[i] = "service-" + string(rune('a'+i%26))
	}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	input := types.GetServicesInput{}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	// Should apply default limit of 100
	assert.Len(t, output.Services, defaultServiceLimit)
}

func TestGetServicesHandler_Error_InvalidPattern(t *testing.T) {
	testServices := []string{"frontend", "payment-service"}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	// Invalid regex pattern
	input := types.GetServicesInput{
		Pattern: "[invalid",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pattern")
	assert.Empty(t, output.Services)
}

func TestGetServicesHandler_Error_StorageFailure(t *testing.T) {
	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return nil, errors.New("storage connection failed")
		},
	}

	handler := &getServicesHandler{queryService: mock}

	input := types.GetServicesInput{}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get services")
	assert.Contains(t, err.Error(), "storage connection failed")
	assert.Empty(t, output.Services)
}

func TestGetServicesHandler_Success_EmptyResult(t *testing.T) {
	// No services in storage
	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return []string{}, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	input := types.GetServicesInput{}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Empty(t, output.Services)
}

func TestGetServicesHandler_Success_NoMatchingPattern(t *testing.T) {
	testServices := []string{"frontend", "payment-service", "cart-service"}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	// Pattern that doesn't match any service
	input := types.GetServicesInput{
		Pattern: "nonexistent",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	assert.Empty(t, output.Services)
}

func TestGetServicesHandler_Success_CaseInsensitivePattern(t *testing.T) {
	testServices := []string{
		"FrontEnd",
		"PAYMENT-SERVICE",
		"cart-service",
	}

	mock := &mockGetServicesQueryService{
		getServicesFunc: func(_ context.Context) ([]string, error) {
			return testServices, nil
		},
	}

	handler := &getServicesHandler{queryService: mock}

	// Case-insensitive pattern
	input := types.GetServicesInput{
		Pattern: "(?i)payment",
	}

	_, output, err := handler.handle(context.Background(), &mcp.CallToolRequest{}, input)

	require.NoError(t, err)
	require.Len(t, output.Services, 1)
	assert.Equal(t, "PAYMENT-SERVICE", output.Services[0])
}

func TestNewGetServicesHandler(t *testing.T) {
	// Test that NewGetServicesHandler returns a valid handler function
	handler := NewGetServicesHandler(nil)
	assert.NotNil(t, handler)
}
