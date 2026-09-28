// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const testDomain = "jaeger.test"

func TestInvalidArgumentStatus_RoundTrip(t *testing.T) {
	tests := []struct {
		err    error
		reason string
	}{
		{err: ErrPaginationInvalid, reason: PaginationInvalidReason},
		{err: ErrSpanOrderInvalid, reason: SpanOrderInvalidReason},
		{err: ErrSpanOrderUnsupported, reason: SpanOrderUnsupportedReason},
	}
	for _, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			wrapped := fmt.Errorf("reader said: %w", test.err)
			assert.Equal(t, test.reason, ErrorReason(wrapped))

			wire := InvalidArgumentStatus(wrapped, testDomain)
			st := status.Convert(wire)
			require.Equal(t, codes.InvalidArgument, st.Code())
			require.Len(t, st.Details(), 1)
			info, ok := st.Details()[0].(*errdetails.ErrorInfo)
			require.True(t, ok)
			assert.Equal(t, test.reason, info.GetReason())
			assert.Equal(t, testDomain, info.GetDomain())

			restored := ErrorFromStatus(wire, testDomain)
			require.ErrorIs(t, restored, test.err)
			assert.Contains(t, restored.Error(), "reader said")
		})
	}
}

func TestInvalidArgumentStatus_PassesOtherErrorsThrough(t *testing.T) {
	assert.Empty(t, ErrorReason(assert.AnError))
	assert.Same(t, assert.AnError, InvalidArgumentStatus(assert.AnError, testDomain))
	assert.Same(t, assert.AnError, ErrorFromStatus(assert.AnError, testDomain))
}

func TestErrorFromStatus_IgnoresForeignDetails(t *testing.T) {
	otherDomain, err := status.New(codes.InvalidArgument, "other").
		WithDetails(&errdetails.ErrorInfo{Reason: PaginationInvalidReason, Domain: "elsewhere"})
	require.NoError(t, err)
	require.NotErrorIs(t, ErrorFromStatus(otherDomain.Err(), testDomain), ErrPaginationInvalid)

	unknownReason, err := status.New(codes.InvalidArgument, "other").
		WithDetails(&errdetails.ErrorInfo{Reason: "SOMETHING_ELSE", Domain: testDomain})
	require.NoError(t, err)
	unknownErr := unknownReason.Err()
	assert.Same(t, unknownErr, ErrorFromStatus(unknownErr, testDomain))

	nonInfo, err := status.New(codes.InvalidArgument, "other").
		WithDetails(&errdetails.DebugInfo{Detail: "noise"})
	require.NoError(t, err)
	nonInfoErr := nonInfo.Err()
	assert.Same(t, nonInfoErr, ErrorFromStatus(nonInfoErr, testDomain))

	// A status with no details at all also passes through, with no wrapping.
	bare := errors.New("bare")
	assert.Same(t, bare, ErrorFromStatus(bare, testDomain))
}
