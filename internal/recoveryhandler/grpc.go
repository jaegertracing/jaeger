// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package recoveryhandler

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NewUnaryServerInterceptor returns a gRPC interceptor that turns a panic in a
// unary call into a codes.Internal error for that call. gRPC does not recover
// handler panics, so without it a panic terminates the whole process.
func NewUnaryServerInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recoveredError(logger, info.FullMethod, r)
			}
		}()
		return handler(ctx, req)
	}
}

// NewStreamServerInterceptor is the streaming counterpart of NewUnaryServerInterceptor.
func NewStreamServerInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = recoveredError(logger, info.FullMethod, r)
			}
		}()
		return handler(srv, ss)
	}
}

// recoveredError logs the panic with the stack where it was raised, which is
// still on the goroutine while the deferred recover runs. The client gets a
// generic message so internal details stay in the server log.
func recoveredError(logger *zap.Logger, method string, r any) error {
	logger.Error("Recovered from panic in gRPC handler",
		zap.String("method", method),
		zap.Any("panic", r),
		zap.Stack("stack"),
	)
	return status.Error(codes.Internal, "internal server error")
}
