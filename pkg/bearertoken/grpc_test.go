// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestBearerTokenInterceptors(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		expectedErr string
		expectedMD  metadata.MD
	}{
		{
			name:        "no token in context",
			ctx:         context.Background(),
			expectedErr: "",
			expectedMD:  nil, // Expecting no metadata
		},
		{
			name:        "token in context",
			ctx:         ContextWithBearerToken(context.Background(), "test-token"),
			expectedErr: "",
			expectedMD:  metadata.MD{Key: []string{"test-token"}},
		},
		{
			name:        "multiple tokens in metadata",
			ctx:         metadata.NewIncomingContext(context.Background(), metadata.MD{Key: []string{"token1", "token2"}}),
			expectedErr: "malformed token: multiple tokens found",
		},
		{
			name:        "valid token in metadata",
			ctx:         metadata.NewIncomingContext(context.Background(), metadata.MD{Key: []string{"valid-token"}}),
			expectedErr: "",
			expectedMD:  metadata.MD{Key: []string{"valid-token"}}, // Valid token setup
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Unary interceptor test
			unaryInterceptor := NewUnaryClientInterceptor()
			unaryInvoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
				md, ok := metadata.FromOutgoingContext(ctx)
				if test.expectedMD == nil {
					require.False(t, ok) // There should be no metadata in this case
				} else {
					require.True(t, ok)
					assert.Equal(t, test.expectedMD, md)
				}
				return nil
			}
			err := unaryInterceptor(test.ctx, "method", nil, nil, nil, unaryInvoker)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedErr)
			}

			// Stream interceptor test
			streamInterceptor := NewStreamClientInterceptor()
			streamInvoker := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
				md, ok := metadata.FromOutgoingContext(ctx)
				if test.expectedMD == nil {
					require.False(t, ok) // There should be no metadata in this case
				} else {
					require.True(t, ok)
					assert.Equal(t, test.expectedMD, md)
				}
				return nil, nil
			}
			_, err = streamInterceptor(test.ctx, &grpc.StreamDesc{}, nil, "method", streamInvoker)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedErr)
			}
		})
	}
}

func TestClientUnaryInterceptorWithBearerToken(t *testing.T) {
	interceptor := NewUnaryClientInterceptor()

	// Mock invoker
	invoker := func(ctx context.Context, _ string, _ any, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		assert.Equal(t, "test-token", md[Key][0])
		return nil
	}

	// Context with token
	ctx := ContextWithBearerToken(context.Background(), "test-token")

	err := interceptor(ctx, "method", nil, nil, nil, invoker)
	require.NoError(t, err)
}

func TestClientStreamInterceptorWithBearerToken(t *testing.T) {
	interceptor := NewStreamClientInterceptor()

	// Mock streamer
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		md, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		assert.Equal(t, "test-token", md[Key][0])
		return nil, nil
	}

	// Context with token
	ctx := ContextWithBearerToken(context.Background(), "test-token")

	_, err := interceptor(ctx, &grpc.StreamDesc{}, nil, "method", streamer)
	require.NoError(t, err)
}

func TestMalformedToken(t *testing.T) {
	// Context with multiple tokens
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{
		Key: []string{"token1", "token2"},
	})

	// Unary interceptor
	unaryInterceptor := NewUnaryClientInterceptor()
	unaryInvoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}
	err := unaryInterceptor(ctx, "method", nil, nil, nil, unaryInvoker)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed token: multiple tokens found")

	// Stream interceptor
	streamInterceptor := NewStreamClientInterceptor()
	streamInvoker := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, nil
	}
	_, err = streamInterceptor(ctx, &grpc.StreamDesc{}, nil, "method", streamInvoker)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed token: multiple tokens found")
}
