// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func StringToSpanKind(sk string) ptrace.SpanKind {
	switch sk {
	case "unspecified":
		return ptrace.SpanKindUnspecified
	case "internal":
		return ptrace.SpanKindInternal
	case "server":
		return ptrace.SpanKindServer
	case "client":
		return ptrace.SpanKindClient
	case "producer":
		return ptrace.SpanKindProducer
	case "consumer":
		return ptrace.SpanKindConsumer
	default:
		return ptrace.SpanKindUnspecified
	}
}

func SpanKindToString(sk ptrace.SpanKind) string {
	return strings.ToLower(sk.String())
}
