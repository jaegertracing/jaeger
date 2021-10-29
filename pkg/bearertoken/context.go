package bearertoken

import "context"

type contextKey string

// Key is the string literal used internally in the implementation of this context.
const Key = "bearer.token"
const bearerToken = contextKey(Key)

// StoragePropagationKey is a key for viper configuration to pass this option to storage plugins.
const StoragePropagationKey = "storage.propagate.token"

// ContextWithBearerToken set bearer token in context.
func ContextWithBearerToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerToken, token)
}

// GetBearerToken from context, or empty string if there is no token.
func GetBearerToken(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(bearerToken).(string)
	return val, ok
}
