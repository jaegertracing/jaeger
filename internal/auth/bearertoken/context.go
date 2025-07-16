// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import "context"

type contextKeyType int

const contextKey = contextKeyType(iota)

// StoragePropagationKey is a key for viper configuration to pass this option to storage plugins.
const StoragePropagationKey = "storage.propagate.token"

// ContextWithBearerToken set bearer token in context.
func ContextWithBearerToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, contextKey, token)
}

// GetBearerToken from context, or empty string if there is no token.
func GetBearerToken(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(contextKey).(string)
	return val, ok
}
