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

func TestRefusalStatus_RoundTrip(t *testing.T) {
	tests := []struct {
		err    error
		reason string
		code   codes.Code
	}{
		{err: ErrPaginationInvalid, reason: PaginationInvalidReason, code: codes.InvalidArgument},
		{err: ErrSpanOrderInvalid, reason: SpanOrderInvalidReason, code: codes.InvalidArgument},
		{err: ErrSpanOrderUnsupported, reason: SpanOrderUnsupportedReason, code: codes.Unimplemented},
	}
	for _, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			wrapped := fmt.Errorf("reader said: %w", test.err)
			assert.Equal(t, test.reason, ErrorReason(wrapped))

			wire := RefusalStatus(wrapped, testDomain)
			st := status.Convert(wire)
			require.Equal(t, test.code, st.Code())
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

func TestRefusalStatus_PassesOtherErrorsThrough(t *testing.T) {
	assert.Empty(t, ErrorReason(assert.AnError))
	assert.Same(t, assert.AnError, RefusalStatus(assert.AnError, testDomain))
	assert.Same(t, assert.AnError, ErrorFromStatus(assert.AnError, testDomain))
}

// TestRefusalStatus_UnsupportedWithoutReason covers a capability refusal that has no reason of
// its own: the code alone tells the client that the backend cannot serve the query.
func TestRefusalStatus_UnsupportedWithoutReason(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("bare reader: %w", errors.ErrUnsupported),
		ErrFilterUnsupported,
		ErrPaginationUnsupported,
	} {
		st := status.Convert(RefusalStatus(err, testDomain))
		assert.Equal(t, codes.Unimplemented, st.Code())
		assert.Empty(t, st.Details())
		assert.Equal(t, err.Error(), st.Message())
	}
}

func TestRefusalCode(t *testing.T) {
	assert.Equal(t, codes.Unimplemented, RefusalCode(ErrFilterUnsupported))
	assert.Equal(t, codes.Unimplemented, RefusalCode(ErrPaginationUnsupported))
	assert.Equal(t, codes.Unimplemented, RefusalCode(ErrSpanOrderUnsupported))
	assert.Equal(t, codes.InvalidArgument, RefusalCode(ErrFilterInvalid))
	assert.Equal(t, codes.InvalidArgument, RefusalCode(ErrPaginationInvalid))
	assert.Equal(t, codes.InvalidArgument, RefusalCode(ErrSpanOrderInvalid))
	assert.Equal(t, codes.InvalidArgument, RefusalCode(ErrPaginationUnsupportedByFindTraces),
		"FindTraces cannot be paginated anywhere, so the caller must change the query")
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
