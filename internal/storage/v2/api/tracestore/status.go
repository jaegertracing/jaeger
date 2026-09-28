// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The ErrorInfo reasons that mark a reader refusal on a status. The status code carries only the
// refusal's family (RefusalCode), and a gRPC server answers the same codes to other failures too,
// so a client needs the reason, not the code, to restore the matching reader error. A refusal
// that carries a reason names a problem with the query itself, as opposed to a bare
// errors.ErrUnsupported, which says only that the reader lacks the method.
const (
	FilterInvalidReason         = "FILTER_INVALID"
	FilterUnsupportedReason     = "FILTER_UNSUPPORTED"
	PaginationInvalidReason     = "PAGINATION_INVALID"
	PaginationUnsupportedReason = "PAGINATION_UNSUPPORTED"
	SpanOrderInvalidReason      = "ORDERING_INVALID"
	SpanOrderUnsupportedReason  = "ORDERING_UNSUPPORTED"
)

// reasonErrors pairs each reason with the reader error it names, in the order ErrorReason
// consults them.
var reasonErrors = []struct {
	reason string
	err    error
}{
	{FilterInvalidReason, ErrFilterInvalid},
	{FilterUnsupportedReason, ErrFilterUnsupported},
	{PaginationInvalidReason, ErrPaginationInvalid},
	{PaginationUnsupportedReason, ErrPaginationUnsupported},
	{SpanOrderInvalidReason, ErrSpanOrderInvalid},
	{SpanOrderUnsupportedReason, ErrSpanOrderUnsupported},
}

// ErrorReason returns the ErrorInfo reason for a reader refusal, or an empty string when err is
// not one.
func ErrorReason(err error) string {
	for _, entry := range reasonErrors {
		if errors.Is(err, entry.err) {
			return entry.reason
		}
	}
	return ""
}

// IsRefusal reports whether err refuses the query rather than failing to serve it: it matches
// ErrInvalidQuery, so the query is malformed, or errors.ErrUnsupported, so this backend lacks a
// capability the query needs. Anything else is a server fault.
func IsRefusal(err error) bool {
	return errors.Is(err, ErrInvalidQuery) || errors.Is(err, errors.ErrUnsupported)
}

// RefusalCode returns the gRPC code for a refusal. A capability this backend lacks is
// Unimplemented, because the same query is valid elsewhere and the caller may fall back; a
// malformed query is InvalidArgument, because the caller must change it wherever it is sent.
func RefusalCode(err error) codes.Code {
	if errors.Is(err, errors.ErrUnsupported) {
		return codes.Unimplemented
	}
	return codes.InvalidArgument
}

// RefusalStatus converts a refusal into a status whose code is RefusalCode and which carries
// the refusal's reason, when it has one, in the given ErrorInfo domain. An error that is not a
// refusal is returned unchanged.
func RefusalStatus(err error, domain string) error {
	if !IsRefusal(err) {
		return err
	}
	reason := ErrorReason(err)
	st := status.New(RefusalCode(err), err.Error())
	if reason == "" {
		return st.Err()
	}
	if detailed, detailErr := st.WithDetails(&errdetails.ErrorInfo{Reason: reason, Domain: domain}); detailErr == nil {
		st = detailed
	}
	return st.Err()
}

// ErrorFromStatus is the inverse of RefusalStatus: it restores the reader error that a status
// with a reason in the given domain names, and reads any other Unimplemented status, whether the
// server lacks the method or refused a capability without a reason, as errors.ErrUnsupported.
// Any other error is returned unchanged.
func ErrorFromStatus(err error, domain string) error {
	st := status.Convert(err)
	for _, detail := range st.Details() {
		info, ok := detail.(*errdetails.ErrorInfo)
		if !ok || info.GetDomain() != domain {
			continue
		}
		for _, entry := range reasonErrors {
			if entry.reason == info.GetReason() {
				return restore(entry.err, st.Message())
			}
		}
	}
	if st.Code() == codes.Unimplemented {
		// A reader that wraps errors.ErrUnsupported itself already ends its message with the
		// root's text, and one that returns the bare root has nothing else to say.
		msg := strings.TrimSuffix(strings.TrimSuffix(st.Message(), errors.ErrUnsupported.Error()), ": ")
		if msg == "" {
			return fmt.Errorf("remote server: %w", errors.ErrUnsupported)
		}
		return fmt.Errorf("remote server: %s: %w", msg, errors.ErrUnsupported)
	}
	return err
}

// restore wraps target so the message reads as the server sent it. RefusalStatus puts the
// error's own text in the status message, so the target's text is not repeated when the message
// already starts with it.
func restore(target error, message string) error {
	if rest, found := strings.CutPrefix(message, target.Error()); found {
		return fmt.Errorf("%w%s", target, rest)
	}
	return fmt.Errorf("%w: %s", target, message)
}
