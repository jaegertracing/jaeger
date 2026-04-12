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

// ProtoSpanKindToString converts a proto-style span kind string
// (e.g. "SPAN_KIND_SERVER") to the lowercase form (e.g. "server").
func ProtoSpanKindToString(s string) string {
	switch s {
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

// StringToProtoSpanKind converts a lowercase span kind string
// (e.g. "server") to the proto-style form (e.g. "SPAN_KIND_SERVER").
// Returns empty string for unknown inputs.
func StringToProtoSpanKind(s string) string {
	switch s {
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
