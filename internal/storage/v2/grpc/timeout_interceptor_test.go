// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestNewTimeoutUnaryClientInterceptor(t *testing.T) {
	interceptor := newTimeoutUnaryClientInterceptor(20 * time.Millisecond)
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		<-ctx.Done()
		return ctx.Err()
	}
	err := interceptor(context.Background(), "method", nil, nil, nil, invoker)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNewTimeoutUnaryClientInterceptor_Success(t *testing.T) {
	interceptor := newTimeoutUnaryClientInterceptor(time.Second)
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		_, ok := ctx.Deadline()
		assert.True(t, ok, "context passed to invoker should carry the configured timeout as a deadline")
		return nil
	}
	err := interceptor(context.Background(), "method", nil, nil, nil, invoker)
	require.NoError(t, err)
}

func TestNewTimeoutStreamClientInterceptor(t *testing.T) {
	t.Run("streamer error cancels the timeout context", func(t *testing.T) {
		interceptor := newTimeoutStreamClientInterceptor(time.Second)
		streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
			return nil, assert.AnError
		}
		stream, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "method", streamer)
		require.Nil(t, stream)
		require.ErrorIs(t, err, assert.AnError)
	})

	t.Run("streamer success wraps the stream with the timeout context", func(t *testing.T) {
		interceptor := newTimeoutStreamClientInterceptor(time.Second)
		fake := &fakeClientStream{}
		streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "context passed to streamer should carry the configured timeout as a deadline")
			require.WithinDuration(t, time.Now().Add(time.Second), deadline, 200*time.Millisecond)
			return fake, nil
		}
		stream, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "method", streamer)
		require.NoError(t, err)
		wrapped, ok := stream.(*timeoutClientStream)
		require.True(t, ok)
		require.Same(t, fake, wrapped.ClientStream)
	})
}

type fakeClientStream struct {
	recvErr  error
	sendErr  error
	closeErr error
}

func (*fakeClientStream) Header() (metadata.MD, error) { return nil, nil }
func (*fakeClientStream) Trailer() metadata.MD         { return nil }
func (*fakeClientStream) Context() context.Context     { return context.Background() }
func (f *fakeClientStream) CloseSend() error           { return f.closeErr }
func (f *fakeClientStream) SendMsg(any) error          { return f.sendErr }
func (f *fakeClientStream) RecvMsg(any) error          { return f.recvErr }

func TestTimeoutClientStream(t *testing.T) {
	tests := []struct {
		name       string
		underlying *fakeClientStream
		call       func(s *timeoutClientStream) error
		wantCancel bool
	}{
		{
			name:       "RecvMsg success does not cancel",
			underlying: &fakeClientStream{},
			call:       func(s *timeoutClientStream) error { return s.RecvMsg(nil) },
			wantCancel: false,
		},
		{
			name:       "RecvMsg error cancels",
			underlying: &fakeClientStream{recvErr: assert.AnError},
			call:       func(s *timeoutClientStream) error { return s.RecvMsg(nil) },
			wantCancel: true,
		},
		{
			name:       "SendMsg success does not cancel",
			underlying: &fakeClientStream{},
			call:       func(s *timeoutClientStream) error { return s.SendMsg(nil) },
			wantCancel: false,
		},
		{
			name:       "SendMsg error cancels",
			underlying: &fakeClientStream{sendErr: assert.AnError},
			call:       func(s *timeoutClientStream) error { return s.SendMsg(nil) },
			wantCancel: true,
		},
		{
			name:       "CloseSend success does not cancel",
			underlying: &fakeClientStream{},
			call:       func(s *timeoutClientStream) error { return s.CloseSend() },
			wantCancel: false,
		},
		{
			name:       "CloseSend error cancels",
			underlying: &fakeClientStream{closeErr: assert.AnError},
			call:       func(s *timeoutClientStream) error { return s.CloseSend() },
			wantCancel: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cancelled := false
			s := &timeoutClientStream{ClientStream: test.underlying, cancel: func() { cancelled = true }}
			_ = test.call(s)
			assert.Equal(t, test.wantCancel, cancelled)
		})
	}
}
