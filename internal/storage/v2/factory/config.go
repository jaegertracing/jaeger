// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// TraceStorageTypeEnvVar is the name of the env var that defines the type of backend used for trace storage.
	TraceStorageTypeEnvVar = "TRACE_STORAGE_TYPE"
)

// Config tells the Factory which types of backends it needs to create for different storage types.
type Config struct {
	TraceWriterTypes []string
}

// ConfigFromEnv reads the desired types of storage backends from TRACE_STORAGE_TYPE environment variables.
// * `clickhouse` - built-in
func ConfigFromEnv(log io.Writer) Config {
	traceWriterType := os.Getenv(TraceStorageTypeEnvVar)
	if traceWriterType == "" {
		return Config{}
	}
	traceWriterTypes := strings.Split(traceWriterType, ",")
	if len(traceWriterTypes) > 1 {
		fmt.Fprintf(log,
			"WARNING: multiple trace storage types have been specified. "+
				"Only the first type (%s) will be used for reading and archiving.\n\n",
			traceWriterTypes[0],
		)
	}
	return Config{TraceWriterTypes: traceWriterTypes}
}
