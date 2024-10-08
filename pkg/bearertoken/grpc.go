// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type tokenatedServerStream struct {
	grpc.ServerStream
	context context.Context
}

func (tss *tokenatedServerStream) Context() context.Context {
	return tss.context
}

func getValidBearerToken(ctx context.Context, bearerHeader string) (string, error) {
	bearerToken, ok := GetBearerToken(ctx)
	if ok && bearerToken != "" {
		return bearerToken, nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.PermissionDenied, "missing bearer token header")
	}

	var err error
	bearerToken, err = tokenFromMetadata(md, bearerHeader)
	if err != nil {
		return "", err
	}
	if bearerToken == "" {
		return bearerToken, status.Errorf(codes.PermissionDenied, "missing bearer token header")
	}

	return bearerToken, nil
}

func tokenFromMetadata(md metadata.MD, bearerHeader string) (string, error) {
	bearerToken := md.Get(bearerHeader)
	if len(bearerToken) < 1 {
		return "", status.Errorf(codes.Unauthenticated, "missing bearer token header")
	} else if len(bearerToken) > 1 {
		return "", status.Errorf(codes.PermissionDenied, "extra bearer token header")
	}

	return bearerToken[0], nil
}

func directlyAttachedBearerToken(ctx context.Context) bool {
	bearerToken, _ := GetBearerToken(ctx)
	return bearerToken != ""
}

func NewGuardingStreamInterceptor(bt string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		bearerToken, err := getValidBearerToken(ss.Context(), bt)
		if err != nil {
			return err
		}

		if directlyAttachedBearerToken(ss.Context()) {
			return handler(srv, ss)
		}

		// "upgrade" the beaerer token to be part of the context, rather than just incoming metadata
		return handler(srv, &tokenatedServerStream{
			ServerStream: ss,
			context:      ContextWithBearerToken(ss.Context(), bearerToken),
		})
	}
}

func NewGuardingUnaryInterceptor(bt string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		bearerToken, err := getValidBearerToken(ctx, bt)
		if err != nil {
			return nil, err
		}

		if directlyAttachedBearerToken(ctx) {
			return handler(ctx, req)
		}

		return handler(ContextWithBearerToken(ctx, bearerToken), req)
	}
}

// NewClientUnaryInterceptor injects bearer token header into gRPC request metadata.
func NewClientUnaryInterceptor(bt string) grpc.UnaryClientInterceptor {
	return grpc.UnaryClientInterceptor(func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if bearerToken, _ := GetBearerToken(ctx); bearerToken != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, bt, bearerToken)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	})
}

// NewClientStreamInterceptor injects bearer token header into gRPC request metadata.
func NewClientStreamInterceptor(bt string) grpc.StreamClientInterceptor {
	return grpc.StreamClientInterceptor(func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if bearerToken, _ := GetBearerToken(ctx); bearerToken != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, bt, bearerToken)
		}
		return streamer(ctx, desc, cc, method, opts...)
	})
}
