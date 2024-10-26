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

func TestClientInterceptors(t *testing.T) {
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
			verifyMetadata := func(ctx context.Context) error {
				md, ok := metadata.FromOutgoingContext(ctx)
				if test.expectedMD == nil {
					require.False(t, ok, "metadata should not be present")
				} else {
					require.True(t, ok, "metadata should be present")
					assert.Equal(t, test.expectedMD, md)
				}
				return nil
			}
			unaryInterceptor := NewUnaryClientInterceptor()
			unaryInvoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
				return verifyMetadata(ctx)
			}
			err := unaryInterceptor(test.ctx, "method", nil, nil, nil, unaryInvoker)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedErr)
			}

			// Stream interceptor test
			streamInterceptor := NewStreamClientInterceptor()
			streamInvoker := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
				if err := verifyMetadata(ctx); err != nil {
					return nil, err
				}
				return nil, nil
			}
			_, err = streamInterceptor(test.ctx, &grpc.StreamDesc{}, nil, "method", streamInvoker)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, test.expectedErr)
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
			verifyToken := func(ctx context.Context) error {
				token, ok := GetBearerToken(ctx)
				if test.wantToken == "" {
					assert.False(t, ok, "expected no token")
				} else {
					assert.True(t, ok, "expected token to be present")
					assert.Equal(t, test.wantToken, token)
				}
				return nil
			}
			// Test unary server interceptor
			unaryInterceptor := NewUnaryServerInterceptor()
			unaryHandler := func(ctx context.Context, _ any) (any, error) {
				return nil, verifyToken(ctx)
			}

			_, err := unaryInterceptor(test.ctx, nil, &grpc.UnaryServerInfo{}, unaryHandler)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedErr)
			}

			// Test stream server interceptor
			streamInterceptor := NewStreamServerInterceptor()
			mockStream := &mockServerStream{ctx: test.ctx}
			streamHandler := func(_ any, stream grpc.ServerStream) error {
				return verifyToken(stream.Context())
			}

			err = streamInterceptor(nil, mockStream, &grpc.StreamServerInfo{}, streamHandler)
			if test.expectedErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorContains(t, err, test.expectedErr)
			}
		})
	}
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
