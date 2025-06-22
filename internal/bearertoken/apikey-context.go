// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import "context"

// APIKeyContextKey is the type used as a key for storing API keys in context.
type apiKeyContextKey struct{}

// APIKeyFromContext retrieves the API key from the context.
func APIKeyFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(apiKeyContextKey{})
	if val == nil {
		return "", false
	}
	if apiKey, ok := val.(string); ok {
		return apiKey, true
	}
	return "", false
}

// ContextWithAPIKey sets the API key in the context if the key is non-empty.
func ContextWithAPIKey(ctx context.Context, apiKey string) context.Context {
	if apiKey == "" {
		return ctx
	}
	return context.WithValue(ctx, apiKeyContextKey{}, apiKey)
}
