// Copyright (c) 2021 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bearertoken

import (
	"context"
)

type contextKeyType int

const (
	bearerTokenContextKey = contextKeyType(iota)
	tenantHeaderContextKey
)

// StoragePropagationKey is a key for viper configuration to pass this option to storage plugins.
const StoragePropagationKey = "storage.propagate.token"

// ContextWithBearerToken set bearer token in context.
func ContextWithBearerToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerTokenContextKey, token)
}

// GetBearerToken from context, or empty string if there is no token.
func GetBearerToken(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(bearerTokenContextKey).(string)
	return val, ok
}

// ContextWithTenant sets tenant into context.
func ContextWithTenant(ctx context.Context, tenant string) context.Context {
	if tenant == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantHeaderContextKey, tenant)
}

// GetTenant returns tenant, or empty string if there is no tenant.
func GetTenant(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(tenantHeaderContextKey).(string)
	return val, ok
}
