// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding

import (
	"context"

	"go.uber.org/zap"
)

type contextKeyType int

const contextKey = contextKeyType(iota)

// CapturedHeader pairs a runtime header value with the config entry that described
// it. In-process consumers retrieve it via CapturedFromContext to read the value
// together with its configured Role and names.
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

func logCapturedHeaders(logger *zap.Logger, protocol string, path string, captured []CapturedHeader) {
	fields := make([]zap.Field, 0, len(captured)+2)
	fields = append(fields, zap.String("protocol", protocol), zap.String("path", path))
	for _, c := range captured {
		fields = append(fields, zap.String("header."+c.Header.HTTPName, c.Value))
	}
	logger.Debug("captured forwarded headers", fields...)
}
