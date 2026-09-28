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

// The ErrorInfo reasons that mark a reader refusal on an InvalidArgument status. A gRPC server
// answers InvalidArgument to other malformed requests too, so a client needs the reason, not the
// code, to restore the matching reader error.
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

// InvalidArgumentStatus converts a reader refusal into an InvalidArgument status that carries
// the refusal's reason in the given ErrorInfo domain. Any other error is returned unchanged.
func InvalidArgumentStatus(err error, domain string) error {
	reason := ErrorReason(err)
	if reason == "" {
		return err
	}
	st := status.New(codes.InvalidArgument, err.Error())
	if detailed, detailErr := st.WithDetails(&errdetails.ErrorInfo{Reason: reason, Domain: domain}); detailErr == nil {
		st = detailed
	}
	return st.Err()
}

// ErrorFromStatus restores the reader error that a status produced by InvalidArgumentStatus in
// the given domain carries. Any other error is returned unchanged.
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
