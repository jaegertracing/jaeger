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

import "context"

type contextKeyType string

const (
	// Key is the key name for the bearer token context value.
	Key = "bearer.token"

	// StoragePropagationKey is a key for viper configuration to pass this option to storage plugins.
	StoragePropagationKey = "storage.propagate.token"

	contextKey = contextKeyType(Key)
)

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
