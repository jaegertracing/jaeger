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
		{err: ErrFilterInvalid, reason: FilterInvalidReason, code: codes.InvalidArgument},
		{err: ErrFilterUnsupported, reason: FilterUnsupportedReason, code: codes.Unimplemented},
		{err: ErrPaginationInvalid, reason: PaginationInvalidReason, code: codes.InvalidArgument},
		{err: ErrPaginationUnsupported, reason: PaginationUnsupportedReason, code: codes.Unimplemented},
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

// TestErrorFromStatus_DoesNotRepeatTheSentinel pins the message a caller reads after the round
// trip: RefusalStatus puts the error's own text in the status message, so the restored error
// starts with the sentinel's text exactly once.
func TestErrorFromStatus_DoesNotRepeatTheSentinel(t *testing.T) {
	sent := fmt.Errorf("%w: token belongs to another query", ErrPaginationInvalid)
	restored := ErrorFromStatus(RefusalStatus(sent, testDomain), testDomain)
	require.ErrorIs(t, restored, ErrPaginationInvalid)
	assert.Equal(t, "invalid pagination: token belongs to another query", restored.Error(),
		"the sentinel carries no root text of its own")

	// A reader that lacks the method wraps errors.ErrUnsupported itself, so its text already
	// ends in the root's; the restored error carries that text once.
	bare := fmt.Errorf("no native summaries: %w", errors.ErrUnsupported)
	restored = ErrorFromStatus(RefusalStatus(bare, testDomain), testDomain)
	require.ErrorIs(t, restored, errors.ErrUnsupported)
	assert.Equal(t, "remote server: no native summaries: unsupported operation", restored.Error())

	restored = ErrorFromStatus(RefusalStatus(errors.ErrUnsupported, testDomain), testDomain)
	require.ErrorIs(t, restored, errors.ErrUnsupported)
	assert.Equal(t, "remote server: unsupported operation", restored.Error(),
		"the bare root has nothing to say beyond itself")
}

// TestEveryUnsupportedReaderSentinelHasAReason guards the rule the query service's summaries
// fallback depends on: a capability refusal a reader can return crosses the storage boundary
// with a reason, so the client never mistakes it for a reader that lacks the method.
func TestEveryUnsupportedReaderSentinelHasAReason(t *testing.T) {
	for _, err := range []error{ErrFilterUnsupported, ErrPaginationUnsupported, ErrSpanOrderUnsupported} {
		require.ErrorIs(t, err, errors.ErrUnsupported)
		assert.NotEmpty(t, ErrorReason(err), err.Error())
	}
}

// TestFamilySentinelMessages pins that a family sentinel reads as its own message: the root it
// unwraps to decides the status code and never appears in the text a caller sees.
func TestFamilySentinelMessages(t *testing.T) {
	assert.Equal(t, "invalid query filter", ErrFilterInvalid.Error())
	require.ErrorIs(t, ErrFilterInvalid, ErrInvalidQuery)
	assert.Equal(t, "this storage backend cannot serve this query filter", ErrFilterUnsupported.Error())
	require.ErrorIs(t, ErrFilterUnsupported, errors.ErrUnsupported)
}

func TestRefusalStatus_PassesOtherErrorsThrough(t *testing.T) {
	assert.Empty(t, ErrorReason(assert.AnError))
	assert.Same(t, assert.AnError, RefusalStatus(assert.AnError, testDomain))
	assert.Same(t, assert.AnError, ErrorFromStatus(assert.AnError, testDomain))
}

// TestRefusalStatus_UnsupportedWithoutReason covers a reader that lacks the method altogether,
// which has no reason of its own: the code alone tells the client the backend cannot serve it.
func TestRefusalStatus_UnsupportedWithoutReason(t *testing.T) {
	err := fmt.Errorf("bare reader: %w", errors.ErrUnsupported)
	st := status.Convert(RefusalStatus(err, testDomain))
	assert.Equal(t, codes.Unimplemented, st.Code())
	assert.Empty(t, st.Details())
	assert.Equal(t, err.Error(), st.Message())
}

func TestIsRefusal(t *testing.T) {
	assert.True(t, IsRefusal(fmt.Errorf("%w: page size", ErrPaginationInvalid)))
	assert.True(t, IsRefusal(fmt.Errorf("%w: level scope", ErrFilterUnsupported)))
	assert.True(t, IsRefusal(fmt.Errorf("no such method: %w", errors.ErrUnsupported)))
	assert.False(t, IsRefusal(assert.AnError), "a server fault is not a refusal")
	assert.False(t, IsRefusal(nil))
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
