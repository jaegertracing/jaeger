// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding

import "context"

type contextKeyType int

const contextKey = contextKeyType(iota)

// CapturedHeader pairs a runtime value with the config entry that described it.
type CapturedHeader struct {
	Header *ForwardedHeader
	Value  string
}

// ContextWithCaptured stores captured header pairs in the context.
func ContextWithCaptured(ctx context.Context, captured []CapturedHeader) context.Context {
	if len(captured) == 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKey, captured)
}

// CapturedFromContext retrieves captured header pairs from the context.
func CapturedFromContext(ctx context.Context) []CapturedHeader {
	if v, ok := ctx.Value(contextKey).([]CapturedHeader); ok {
		return v
	}
	return nil
}
