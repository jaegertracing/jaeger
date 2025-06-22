// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIKeyFromContext(t *testing.T) {
	// No value in context
	emptyCtx := context.Background()
	apiKey, ok := APIKeyFromContext(emptyCtx)
	assert.Empty(t, apiKey)
	assert.False(t, ok)

	// Correct string value in context (via ContextWithAPIKey)
	expectedApiKey := "test-api-key"
	ctxWithApiKey := ContextWithAPIKey(context.Background(), expectedApiKey)
	apiKey, ok = APIKeyFromContext(ctxWithApiKey)
	assert.Equal(t, expectedApiKey, apiKey)
	assert.True(t, ok)

	// Non-string value in context (simulate misuse)
	ctxWithNonString := context.WithValue(context.Background(), apiKeyContextKey{}, 123)
	apiKey, ok = APIKeyFromContext(ctxWithNonString)
	assert.Empty(t, apiKey)
	assert.False(t, ok)

	// Empty string value in context (should be set, but value is empty)
	emptyStringCtx := ContextWithAPIKey(context.Background(), "")
	apiKey, ok = APIKeyFromContext(emptyStringCtx)
	assert.Empty(t, apiKey)
	assert.False(t, ok)
}

func TestContextWithAPIKey(t *testing.T) {
	baseCtx := context.Background()

	// Non-empty apiKey: should set value in context
	apiKey := "my-secret-key"
	ctxWithKey := ContextWithAPIKey(baseCtx, apiKey)
	val, ok := APIKeyFromContext(ctxWithKey)
	assert.True(t, ok, "apiKey should be present in context")
	assert.Equal(t, apiKey, val)

	// Empty apiKey: should return original context, no value set
	emptyCtx := ContextWithAPIKey(baseCtx, "")
	val, ok = APIKeyFromContext(emptyCtx)
	assert.False(t, ok, "apiKey should not be present for empty string")
	assert.Empty(t, val)

	// Should not mutate original context
	val, ok = APIKeyFromContext(baseCtx)
	assert.False(t, ok, "original context should remain unchanged")
	assert.Empty(t, val)
}
