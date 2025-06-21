// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApiKeyFromContext(t *testing.T) {
	// No value in context
	emptyCtx := context.Background()
	apiKey, ok := ApiKeyFromContext(emptyCtx)
	assert.Empty(t, apiKey)
	assert.False(t, ok)

	// Correct string value in context (via ContextWithAPIKey)
	expectedApiKey := "test-api-key"
	ctxWithApiKey := ContextWithAPIKey(context.Background(), expectedApiKey)
	apiKey, ok = ApiKeyFromContext(ctxWithApiKey)
	assert.Equal(t, expectedApiKey, apiKey)
	assert.True(t, ok)

	// Non-string value in context (simulate misuse)
	ctxWithNonString := context.WithValue(context.Background(), APIKeyContextKey{}, 123)
	apiKey, ok = ApiKeyFromContext(ctxWithNonString)
	assert.Empty(t, apiKey)
	assert.False(t, ok)

	// Empty string value in context (should be set, but value is empty)
	emptyStringCtx := ContextWithAPIKey(context.Background(), "")
	apiKey, ok = ApiKeyFromContext(emptyStringCtx)
	assert.Empty(t, apiKey)
	assert.False(t, ok)
}

func TestContextWithAPIKey(t *testing.T) {
	baseCtx := context.Background()

	// Non-empty apiKey: should set value in context
	apiKey := "my-secret-key"
	ctxWithKey := ContextWithAPIKey(baseCtx, apiKey)
	val, ok := ApiKeyFromContext(ctxWithKey)
	assert.True(t, ok, "apiKey should be present in context")
	assert.Equal(t, apiKey, val)

	// Empty apiKey: should return original context, no value set
	emptyCtx := ContextWithAPIKey(baseCtx, "")
	val, ok = ApiKeyFromContext(emptyCtx)
	assert.False(t, ok, "apiKey should not be present for empty string")
	assert.Empty(t, val)

	// Should not mutate original context
	val, ok = ApiKeyFromContext(baseCtx)
	assert.False(t, ok, "original context should remain unchanged")
	assert.Empty(t, val)
}
