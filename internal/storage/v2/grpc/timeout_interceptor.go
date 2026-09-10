// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// newTimeoutUnaryClientInterceptor returns a gRPC unary client interceptor that bounds
// every call by the given timeout, so a slow or hung storage backend fails fast with a
// deadline-exceeded error instead of blocking indefinitely (or until an unrelated parent
// context is cancelled).
func newTimeoutUnaryClientInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// newTimeoutStreamClientInterceptor returns a gRPC streaming client interceptor that bounds
// stream creation and the lifetime of the stream by the given timeout.
func newTimeoutStreamClientInterceptor(timeout time.Duration) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		stream, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			cancel()
			return nil, err
		}
		return &timeoutClientStream{ClientStream: stream, cancel: cancel}, nil
	}
}

// timeoutClientStream wraps grpc.ClientStream so the timeout context is released once the
// stream finishes (its first Send/Recv/CloseSend error, including io.EOF) rather than
// immediately after the streamer call returns, while the stream is still being consumed.
type timeoutClientStream struct {
	grpc.ClientStream
	cancel context.CancelFunc
}

func (s *timeoutClientStream) RecvMsg(m any) error {
	err := s.ClientStream.RecvMsg(m)
	if err != nil {
		s.cancel()
	}
	return err
}

func (s *timeoutClientStream) SendMsg(m any) error {
	err := s.ClientStream.SendMsg(m)
	if err != nil {
		s.cancel()
	}
	return err
}

func (s *timeoutClientStream) CloseSend() error {
	err := s.ClientStream.CloseSend()
	if err != nil {
		s.cancel()
	}
	return err
}
