// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package recoveryhandler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func panickingHandler() {
	panic("handler failed")
}

func assertPanicLogged(t *testing.T, logs *observer.ObservedLogs, method string) {
	t.Helper()
	entries := logs.All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	assert.Equal(t, method, fields["method"])
	assert.Equal(t, "handler failed", fields["panic"])
	// The stack must reach the frame that panicked, not stop at the interceptor.
	assert.Contains(t, fields["stack"], "recoveryhandler.panickingHandler")
}

func TestUnaryServerInterceptorRecoversPanic(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	interceptor := NewUnaryServerInterceptor(zap.New(core))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Unary"}

	resp, err := interceptor(context.Background(), "req", info, func(context.Context, any) (any, error) {
		panickingHandler()
		return "resp", nil
	})

	assert.Nil(t, resp)
	assert.Equal(t, codes.Internal, status.Code(err))
	assertPanicLogged(t, logs, info.FullMethod)
}

func TestUnaryServerInterceptorPassesThrough(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	interceptor := NewUnaryServerInterceptor(zap.New(core))
	handlerErr := errors.New("handler error")

	resp, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{}, func(_ context.Context, req any) (any, error) {
		return req, handlerErr
	})

	assert.Equal(t, "req", resp)
	require.ErrorIs(t, err, handlerErr)
	assert.Zero(t, logs.Len())
}

func TestStreamServerInterceptorRecoversPanic(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	interceptor := NewStreamServerInterceptor(zap.New(core))
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}

	err := interceptor(nil, nil, info, func(any, grpc.ServerStream) error {
		panickingHandler()
		return nil
	})

	assert.Equal(t, codes.Internal, status.Code(err))
	assertPanicLogged(t, logs, info.FullMethod)
}

func TestStreamServerInterceptorPassesThrough(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	interceptor := NewStreamServerInterceptor(zap.New(core))
	handlerErr := errors.New("handler error")

	err := interceptor(nil, nil, &grpc.StreamServerInfo{}, func(any, grpc.ServerStream) error {
		return handlerErr
	})

	require.ErrorIs(t, err, handlerErr)
	assert.Zero(t, logs.Len())
}
