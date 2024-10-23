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

type mockServerStream struct {
	ctx context.Context
	grpc.ServerStream
}

func (s *mockServerStream) Context() context.Context {
	return s.ctx
}

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

func TestServerInterceptors(t *testing.T) {
	tests := []struct {
		name        string
		ctx         context.Context
		expectedErr string
		wantToken   string
	}{
		{
			name:        "no token in context",
			ctx:         context.Background(),
			expectedErr: "",
			wantToken:   "",
		},
		{
			name:        "token in context",
			ctx:         ContextWithBearerToken(context.Background(), "test-token"),
			expectedErr: "",
			wantToken:   "test-token",
		},
		{
			name: "multiple tokens in metadata",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				Key: []string{"token1", "token2"},
			}),
			expectedErr: "malformed token: multiple tokens found",
			wantToken:   "",
		},
		{
			name: "valid token in metadata",
			ctx: metadata.NewIncomingContext(context.Background(), metadata.MD{
				Key: []string{"valid-token"},
			}),
			expectedErr: "",
			wantToken:   "valid-token",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Test unary server interceptor
			unaryInterceptor := NewUnaryServerInterceptor()
			unaryHandler := func(ctx context.Context, _ any) (any, error) {
				token, ok := GetBearerToken(ctx)
				if test.wantToken == "" {
					assert.False(t, ok, "expected no token")
				} else {
					assert.True(t, ok, "expected token to be present")
					assert.Equal(t, test.wantToken, token)
				}
				return nil, nil
			}

			_, err := unaryInterceptor(test.ctx, nil, &grpc.UnaryServerInfo{}, unaryHandler)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.expectedErr)
			}

			// Test stream server interceptor
			streamInterceptor := NewStreamServerInterceptor()
			mockStream := &mockServerStream{ctx: test.ctx}
			streamHandler := func(_ any, stream grpc.ServerStream) error {
				token, ok := GetBearerToken(stream.Context())
				if test.wantToken == "" {
					assert.False(t, ok, "expected no token")
				} else {
					assert.True(t, ok, "expected token to be present")
					assert.Equal(t, test.wantToken, token)
				}
				return nil
			}

			err = streamInterceptor(nil, mockStream, &grpc.StreamServerInfo{}, streamHandler)
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

func TestServerUnaryInterceptorWithBearerToken(t *testing.T) {
	interceptor := NewUnaryServerInterceptor()
	testToken := "test-token"

	// Test with token in context
	handler := func(ctx context.Context, _ any) (any, error) {
		token, ok := GetBearerToken(ctx)
		require.True(t, ok)
		assert.Equal(t, testToken, token)
		return nil, nil
	}

	ctx := ContextWithBearerToken(context.Background(), testToken)
	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)
	require.NoError(t, err)
}

func TestServerStreamInterceptorWithBearerToken(t *testing.T) {
	interceptor := NewStreamServerInterceptor()
	testToken := "test-token"

	// Test with token in context
	handler := func(_ any, stream grpc.ServerStream) error {
		token, ok := GetBearerToken(stream.Context())
		require.True(t, ok)
		assert.Equal(t, testToken, token)
		return nil
	}

	ctx := ContextWithBearerToken(context.Background(), testToken)
	mockStream := &mockServerStream{ctx: ctx}
	err := interceptor(nil, mockStream, &grpc.StreamServerInfo{}, handler)
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

func TestTokenatedServerStream(t *testing.T) {
	originalCtx := context.Background()
	testToken := "test-token"
	newCtx := ContextWithBearerToken(originalCtx, testToken)

	stream := &tokenatedServerStream{
		ServerStream: &mockServerStream{ctx: originalCtx},
		context:      newCtx,
	}

	// Verify that Context() returns the modified context
	token, ok := GetBearerToken(stream.Context())
	require.True(t, ok)
	assert.Equal(t, testToken, token)
}
