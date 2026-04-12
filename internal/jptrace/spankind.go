// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func StringToSpanKind(sk string) ptrace.SpanKind {
	switch strings.ToLower(sk) {
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
	if sk == ptrace.SpanKindUnspecified {
		return ""
	}
	return strings.ToLower(sk.String())
}

func ProtoSpanKindToString(s string) string {
	switch strings.ToUpper(s) {
	case "SPAN_KIND_UNSPECIFIED":
		return "unspecified"
	case "SPAN_KIND_INTERNAL":
		return "internal"
	case "SPAN_KIND_SERVER":
		return "server"
	case "SPAN_KIND_CLIENT":
		return "client"
	case "SPAN_KIND_PRODUCER":
		return "producer"
	case "SPAN_KIND_CONSUMER":
		return "consumer"
	default:
		return ""
	}
}

func StringToProtoSpanKind(s string) string {
	switch strings.ToLower(s) {
	case "unspecified":
		return "SPAN_KIND_UNSPECIFIED"
	case "internal":
		return "SPAN_KIND_INTERNAL"
	case "server":
		return "SPAN_KIND_SERVER"
	case "client":
		return "SPAN_KIND_CLIENT"
	case "producer":
		return "SPAN_KIND_PRODUCER"
	case "consumer":
		return "SPAN_KIND_CONSUMER"
	default:
		return ""
	}
}
