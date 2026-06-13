// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// serverStream wraps grpc.ServerStream to carry an updated context.
type serverStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *serverStream) Context() context.Context {
	return s.ctx
}

func extractFromIncomingMetadata(ctx context.Context, headers []ForwardedHeader) []CapturedHeader {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	captured := make([]CapturedHeader, 0, len(headers))
	for i := range headers {
		if vals := md.Get(headers[i].inboundGRPCName()); len(vals) > 0 {
			captured = append(captured, CapturedHeader{Header: &headers[i], Value: vals[0]})
		}
	}
	return captured
}

// NewUnaryServerInterceptor returns a gRPC unary server interceptor that extracts
// the configured headers from incoming metadata and stores them in the context.
func NewUnaryServerInterceptor(headers []ForwardedHeader) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if incoming := extractFromIncomingMetadata(ctx, headers); len(incoming) > 0 {
			ctx = ContextWithCaptured(ctx, incoming)
		}
		return handler(ctx, req)
	}
}

// NewStreamServerInterceptor returns a gRPC streaming server interceptor that extracts
// the configured headers from incoming metadata and stores them in the context.
func NewStreamServerInterceptor(headers []ForwardedHeader) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		if incoming := extractFromIncomingMetadata(ctx, headers); len(incoming) > 0 {
			ss = &serverStream{ServerStream: ss, ctx: ContextWithCaptured(ctx, incoming)}
		}
		return handler(srv, ss)
	}
}

// NewUnaryClientInterceptor returns a gRPC unary client interceptor that forwards
// header values stored in the context as outgoing metadata to the storage backend.
func NewUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx = appendOutgoing(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// NewStreamClientInterceptor returns a gRPC streaming client interceptor that forwards
// header values stored in the context as outgoing metadata to the storage backend.
func NewStreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = appendOutgoing(ctx)
		return streamer(ctx, desc, cc, method, opts...)
	}
}

func appendOutgoing(ctx context.Context) context.Context {
	for _, c := range CapturedFromContext(ctx) {
		ctx = metadata.AppendToOutgoingContext(ctx, c.Header.outboundGRPCName(), c.Value)
	}
	return ctx
}
