// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"encoding/binary"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// pcommonTraceIDFromBytes converts a remote-storage trace ID to pdata form.
// 16-byte IDs are copied as-is. 8-byte IDs are placed in the low half, matching
// model.TraceIDFromBytes. A raw copy into a 16-byte array would put those bytes
// in the high half and fail to match the stored trace.
func pcommonTraceIDFromBytes(data []byte) (pcommon.TraceID, error) {
	tid, err := model.TraceIDFromBytes(data)
	if err != nil {
		return pcommon.TraceID{}, fmt.Errorf("invalid trace ID: %w", err)
	}
	var out pcommon.TraceID
	binary.BigEndian.PutUint64(out[:8], tid.High)
	binary.BigEndian.PutUint64(out[8:], tid.Low)
	return out, nil
}
