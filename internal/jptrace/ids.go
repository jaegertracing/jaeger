// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"encoding/hex"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// TraceIDFromString decodes a 32-character hex string into a pcommon.TraceID.
// Unlike model.TraceIDFromString, it rejects shorter strings instead of
// zero-padding them, so callers that need lenient parsing of user input
// should go through the v1 model type instead.
func TraceIDFromString(s string) (pcommon.TraceID, error) {
	var id pcommon.TraceID
	if err := decodeHexID(s, id[:], "trace"); err != nil {
		return pcommon.TraceID{}, err
	}
	return id, nil
}

// SpanIDFromString decodes a 16-character hex string into a pcommon.SpanID.
// It accepts the all-zero span ID; callers that cannot use it must check
// IsEmpty themselves.
func SpanIDFromString(s string) (pcommon.SpanID, error) {
	var id pcommon.SpanID
	if err := decodeHexID(s, id[:], "span"); err != nil {
		return pcommon.SpanID{}, err
	}
	return id, nil
}

// decodeHexID fills dst from the hex string s and fails unless the decoded
// bytes fill dst exactly, so a corrupted or truncated ID cannot slip through.
func decodeHexID(s string, dst []byte, kind string) error {
	b, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	if len(b) != len(dst) {
		return fmt.Errorf("invalid length %d of decoded %s ID %q, expected %d bytes", len(b), kind, s, len(dst))
	}
	copy(dst, b)
	return nil
}
