// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
)

// testHeaders has all four fields populated to exercise the full propagation path.
var testHeaders = []headerforwarding.ForwardedHeader{
	{
		HTTPName:         "x-user",
		GRPCName:         "x-grpc-user",
		Role:             headerforwarding.RoleUsername,
		GRPCOutboundName: "x-forwarded-user",
	},
	{
		HTTPName: "x-email",
		Role:     headerforwarding.RoleEmail,
		// GRPCName and GRPCOutboundName both fall back to HTTPName
	},
}

// mockServerStream implements grpc.ServerStream for testing.
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context { return m.ctx }

// capturedToMap converts []CapturedHeader to a map[httpName]value for easy assertion.
func capturedToMap(captured []headerforwarding.CapturedHeader) map[string]string {
	if captured == nil {
		return nil
	}
	m := make(map[string]string, len(captured))
	for _, c := range captured {
		m[c.Header.HTTPName] = c.Value
	}
	return m
}

// --- Server interceptor tests ---

func TestUnaryServerInterceptor_ExtractsFromMetadata(t *testing.T) {
	md := metadata.New(map[string]string{
		"x-grpc-user": "alice",
		"x-email":     "alice@example.com",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	interceptor := headerforwarding.NewUnaryServerInterceptor(testHeaders)
	var gotMap map[string]string
	handler := func(ctx context.Context, _ any) (any, error) {
		gotMap = capturedToMap(headerforwarding.CapturedFromContext(ctx))
		return nil, nil
	}
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"x-user":  "alice",
		"x-email": "alice@example.com",
	}, gotMap)
}

func TestUnaryServerInterceptor_MultiValueUsesFirst(t *testing.T) {
	md := metadata.Pairs("x-grpc-user", "alice", "x-grpc-user", "bob")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	interceptor := headerforwarding.NewUnaryServerInterceptor(testHeaders)
	var gotMap map[string]string
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
		gotMap = capturedToMap(headerforwarding.CapturedFromContext(ctx))
		return nil, nil
	})
	require.NoError(t, err)
	assert.Equal(t, "alice", gotMap["x-user"])
}

func TestUnaryServerInterceptor_NoMetadata(t *testing.T) {
	interceptor := headerforwarding.NewUnaryServerInterceptor(testHeaders)
	var got []headerforwarding.CapturedHeader
	handler := func(ctx context.Context, _ any) (any, error) {
		got = headerforwarding.CapturedFromContext(ctx)
		return nil, nil
	}
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestStreamServerInterceptor_ExtractsFromMetadata(t *testing.T) {
	md := metadata.New(map[string]string{"x-grpc-user": "alice"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	interceptor := headerforwarding.NewStreamServerInterceptor(testHeaders)
	var gotMap map[string]string
	handler := func(_ any, ss grpc.ServerStream) error {
		gotMap = capturedToMap(headerforwarding.CapturedFromContext(ss.Context()))
		return nil
	}
	err := interceptor(nil, &mockServerStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"x-user": "alice"}, gotMap)
}

// --- Client interceptor tests ---

func TestUnaryClientInterceptor_ForwardsHeaders(t *testing.T) {
	ctx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
		{Header: &testHeaders[0], Value: "alice"},
		{Header: &testHeaders[1], Value: "alice@example.com"},
	})

	interceptor := headerforwarding.NewUnaryClientInterceptor()
	var gotMD metadata.MD
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		gotMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}
	err := interceptor(ctx, "method", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice"}, gotMD.Get("x-forwarded-user"))
	assert.Equal(t, []string{"alice@example.com"}, gotMD.Get("x-email"))
}

func TestUnaryClientInterceptor_NoHeaders(t *testing.T) {
	interceptor := headerforwarding.NewUnaryClientInterceptor()
	var gotMD metadata.MD
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		gotMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}
	err := interceptor(context.Background(), "method", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.Empty(t, gotMD)
}

func TestStreamClientInterceptor_ForwardsHeaders(t *testing.T) {
	ctx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
		{Header: &testHeaders[0], Value: "alice"},
	})

	interceptor := headerforwarding.NewStreamClientInterceptor()
	var gotMD metadata.MD
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		gotMD, _ = metadata.FromOutgoingContext(ctx)
		return nil, nil
	}
	_, err := interceptor(ctx, &grpc.StreamDesc{}, nil, "method", streamer)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice"}, gotMD.Get("x-forwarded-user"))
}

// --- Fallback tests ---

func TestHeaderFallbacks(t *testing.T) {
	t.Run("no GRPCName or GRPCOutboundName: all fall back to HTTPName", func(t *testing.T) {
		hdr := headerforwarding.ForwardedHeader{HTTPName: "x-user", Role: headerforwarding.RoleUsername}
		headers := []headerforwarding.ForwardedHeader{hdr}

		md := metadata.New(map[string]string{"x-user": "alice"})
		ctx := metadata.NewIncomingContext(context.Background(), md)

		var gotMap map[string]string
		_, err := headerforwarding.NewUnaryServerInterceptor(headers)(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, _ any) (any, error) {
			gotMap = capturedToMap(headerforwarding.CapturedFromContext(ctx))
			return nil, nil
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"x-user": "alice"}, gotMap)

		clientCtx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
			{Header: &headers[0], Value: "alice"},
		})
		var gotMD metadata.MD
		err = headerforwarding.NewUnaryClientInterceptor()(clientCtx, "m", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			gotMD, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"alice"}, gotMD.Get("x-user"))
	})

	t.Run("GRPCName set, no GRPCOutboundName: outbound falls back to GRPCName", func(t *testing.T) {
		hdr := headerforwarding.ForwardedHeader{HTTPName: "x-user", GRPCName: "x-grpc-user", Role: headerforwarding.RoleUsername}
		clientCtx := headerforwarding.ContextWithCaptured(context.Background(), []headerforwarding.CapturedHeader{
			{Header: &hdr, Value: "alice"},
		})
		var gotMD metadata.MD
		err := headerforwarding.NewUnaryClientInterceptor()(clientCtx, "m", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			gotMD, _ = metadata.FromOutgoingContext(ctx)
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"alice"}, gotMD.Get("x-grpc-user"))
		assert.Empty(t, gotMD.Get("x-user"))
	})
}
