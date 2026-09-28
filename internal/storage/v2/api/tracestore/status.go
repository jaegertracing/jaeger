// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"errors"
	"fmt"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The ErrorInfo reasons that mark a reader refusal on a status. The status code carries only the
// refusal's family (RefusalCode), and a gRPC server answers the same codes to other failures too,
// so a client needs the reason, not the code, to restore the matching reader error.
const (
	PaginationInvalidReason    = "PAGINATION_INVALID"
	SpanOrderInvalidReason     = "ORDERING_INVALID"
	SpanOrderUnsupportedReason = "ORDERING_UNSUPPORTED"
)

// reasonErrors pairs each reason with the reader error it names.
var reasonErrors = map[string]error{
	PaginationInvalidReason:    ErrPaginationInvalid,
	SpanOrderInvalidReason:     ErrSpanOrderInvalid,
	SpanOrderUnsupportedReason: ErrSpanOrderUnsupported,
}

// ErrorReason returns the ErrorInfo reason for a reader refusal, or an empty string when err is
// not one.
func ErrorReason(err error) string {
	for reason, target := range reasonErrors {
		if errors.Is(err, target) {
			return reason
		}
	}
	return ""
}

// RefusalCode returns the gRPC code for a query the reader refused. A refusal that matches
// errors.ErrUnsupported names a capability this backend lacks, so the same query is valid
// elsewhere and the caller may fall back; that is Unimplemented. Every other refusal means the
// query is malformed on its own terms and is InvalidArgument.
func RefusalCode(err error) codes.Code {
	if errors.Is(err, errors.ErrUnsupported) {
		return codes.Unimplemented
	}
	return codes.InvalidArgument
}

// RefusalStatus converts a reader refusal into a status whose code is RefusalCode and which
// carries the refusal's reason, when it has one, in the given ErrorInfo domain. An error that
// neither matches errors.ErrUnsupported nor carries a reason is returned unchanged.
func RefusalStatus(err error, domain string) error {
	reason := ErrorReason(err)
	if reason == "" && !errors.Is(err, errors.ErrUnsupported) {
		return err
	}
	st := status.New(RefusalCode(err), err.Error())
	if reason == "" {
		return st.Err()
	}
	if detailed, detailErr := st.WithDetails(&errdetails.ErrorInfo{Reason: reason, Domain: domain}); detailErr == nil {
		st = detailed
	}
	return st.Err()
}

// ErrorFromStatus restores the reader error that a status produced by RefusalStatus in the
// given domain carries. Any other error is returned unchanged.
func ErrorFromStatus(err error, domain string) error {
	st := status.Convert(err)
	for _, detail := range st.Details() {
		info, ok := detail.(*errdetails.ErrorInfo)
		if !ok || info.GetDomain() != domain {
			continue
		}
		if target, known := reasonErrors[info.GetReason()]; known {
			return fmt.Errorf("%w: %s", target, st.Message())
		}
	}
	return err
}
