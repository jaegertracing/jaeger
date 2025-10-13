// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import "go.opentelemetry.io/collector/pdata/ptrace"

func StringToStatusCode(sc string) ptrace.StatusCode {
	switch sc {
	case "Ok":
		return ptrace.StatusCodeOk
	case "Error":
		return ptrace.StatusCodeError
	default:
		return ptrace.StatusCodeUnset
	}
}
