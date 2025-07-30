// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apikey

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAPIKey(t *testing.T) {
	// No value in context
	emptyCtx := context.Background()
	apiKey, ok := GetAPIKey(emptyCtx)
	assert.Empty(t, apiKey)
	assert.False(t, ok)

	// Correct string value in context (via ContextWithAPIKey)
	expectedApiKey := "test-api-key"
	ctxWithApiKey := ContextWithAPIKey(context.Background(), expectedApiKey)
	apiKey, ok = GetAPIKey(ctxWithApiKey)
	assert.Equal(t, expectedApiKey, apiKey)
	assert.True(t, ok)

	// Non-string value in context (simulate misuse)
	ctxWithNonString := context.WithValue(context.Background(), apiKeyContextKey{}, 123)
	apiKey, ok = GetAPIKey(ctxWithNonString)
	assert.Empty(t, apiKey)
	assert.False(t, ok)

	// Empty string value in context (should be set, but value is empty)
	emptyStringCtx := ContextWithAPIKey(context.Background(), "")
	apiKey, ok = GetAPIKey(emptyStringCtx)
	assert.Empty(t, apiKey)
	assert.False(t, ok)
}

func TestContextWithAPIKey(t *testing.T) {
	baseCtx := context.Background()

	// Non-empty apiKey: should set value in context
	apiKey := "my-secret-key"
	ctxWithKey := ContextWithAPIKey(baseCtx, apiKey)
	val, ok := GetAPIKey(ctxWithKey)
	assert.True(t, ok, "apiKey should be present in context")
	assert.Equal(t, apiKey, val)

	// Empty apiKey: should return original context, no value set
	emptyCtx := ContextWithAPIKey(baseCtx, "")
	val, ok = GetAPIKey(emptyCtx)
	assert.False(t, ok, "apiKey should not be present for empty string")
	assert.Empty(t, val)

	// Should not mutate original context
	val, ok = GetAPIKey(baseCtx)
	assert.False(t, ok, "original context should remain unchanged")
	assert.Empty(t, val)
}
