// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const Key = "bearer.token"

type tokenatedServerStream struct {
	grpc.ServerStream
	context context.Context
}

// getValidBearerToken attempts to retrieve the bearer token from the context.
// It does not return an error if the token is missing.
func getValidBearerToken(ctx context.Context, bearerHeader string) (string, error) {
	bearerToken, ok := GetBearerToken(ctx)
	if ok && bearerToken != "" {
		return bearerToken, nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", nil
	}

	return tokenFromMetadata(md, bearerHeader)
}

// tokenFromMetadata extracts the bearer token from the metadata.
func tokenFromMetadata(md metadata.MD, bearerHeader string) (string, error) {
	bearerToken := md.Get(bearerHeader)
	if len(bearerToken) < 1 {
		return "", nil
	}
	if len(bearerToken) > 1 {
		return "", fmt.Errorf("malformed token: multiple tokens found")
	}
	return bearerToken[0], nil
}

// NewStreamServerInterceptor creates a new stream interceptor that injects the bearer token into the context if available.
func NewStreamServerInterceptor() grpc.StreamServerInterceptor {
	return streamServerInterceptor
}

func streamServerInterceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	bearerToken, err := getValidBearerToken(ss.Context(), Key)
	if err != nil {
		return err
	}
	// Upgrade the bearer token to be part of the context.
	return handler(srv, &tokenatedServerStream{
		ServerStream: ss,
		context:      ContextWithBearerToken(ss.Context(), bearerToken),
	})
}

// NewUnaryServerInterceptor creates a new unary interceptor that injects the bearer token into the context if available.
func NewUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		bearerToken, err := getValidBearerToken(ctx, Key)
		if err != nil {
			return nil, err
		}

		return handler(ContextWithBearerToken(ctx, bearerToken), req)
	}
}

// NewUnaryClientInterceptor injects the bearer token header into gRPC request metadata.
func NewUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return grpc.UnaryClientInterceptor(func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		if len(md[Key]) == 0 {
			bearerToken, err := GetBearerToken(ctx)
			if err {
				return invoker(ctx, method, req, reply, cc, opts...) // Proceed without a token if there's an error
			}

			if bearerToken != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, Key, bearerToken)
			}
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	})
}

// NewStreamClientInterceptor injects the bearer token header into gRPC request metadata.
func NewStreamClientInterceptor() grpc.StreamClientInterceptor {
	return grpc.StreamClientInterceptor(func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		md, _ := metadata.FromOutgoingContext(ctx)
		if len(md[Key]) == 0 {
			bearerToken, err := GetBearerToken(ctx)
			if err {
				return streamer(ctx, desc, cc, method, opts...)
			}

			if bearerToken != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, Key, bearerToken)
			}
		}

		return streamer(ctx, desc, cc, method, opts...)
	})
}
