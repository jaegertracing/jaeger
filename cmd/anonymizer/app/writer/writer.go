// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package writer

import (
	"fmt"
	"os"
	"path/filepath"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

// permUserRW keeps the files readable by their owner only, since the captured one holds the
// trace as it was, before anything was hashed.
const permUserRW = 0o600

// WriteTraces writes the traces to path as one OTLP JSON document, the format Jaeger UI accepts
// as an upload.
func WriteTraces(path string, traces ptrace.Traces) error {
	var marshaler ptrace.JSONMarshaler
	dat, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return fmt.Errorf("cannot marshal traces: %w", err)
	}
	if err := os.WriteFile(filepath.Clean(path), dat, permUserRW); err != nil {
		return fmt.Errorf("cannot write output file: %w", err)
	}
	return nil
}
