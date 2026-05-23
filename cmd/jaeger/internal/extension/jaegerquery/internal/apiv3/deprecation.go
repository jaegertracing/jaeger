// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package apiv3

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
)

const (
	// deprecatedParamsSunset is the RFC 1123 Sunset header value for removal of
	// snake_case query parameter aliases (Jaeger v2.20.0 target).
	deprecatedParamsSunset = "Mon, 01 Nov 2026 00:00:00 GMT"

	deprecatedParamsMigrationURL = "https://github.com/jaegertracing/jaeger/blob/main/docs/apis/api_v3_http.md"
)

// applyDeprecationHeaders sets RFC 8594 / draft deprecation headers when deprecated
// query parameters were used. No-op when deprecated is nil or empty.
func applyDeprecationHeaders(w http.ResponseWriter, deprecated []string) {
	if len(deprecated) == 0 {
		return
	}
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", deprecatedParamsSunset)
	w.Header().Set("Link", "<"+deprecatedParamsMigrationURL+">; rel=\"deprecation\"")
	w.Header().Set("Deprecated-Params", strings.Join(deprecated, ","))
}

// logDeprecatedParams emits one WARN log line per request when deprecated params were used.
func logDeprecatedParams(logger *zap.Logger, remoteAddr string, deprecated []string) {
	if logger == nil || len(deprecated) == 0 {
		return
	}
	canonical := make([]string, len(deprecated))
	for i, dep := range deprecated {
		canonical[i] = canonicalForDeprecated(dep)
	}
	logger.Warn(
		"deprecated API v3 query parameter used",
		zap.Strings("param_name", deprecated),
		zap.Strings("canonical_name", canonical),
		zap.String("remote_addr", remoteAddr),
	)
}

// canonicalForDeprecated maps a deprecated param name to its canonical camelCase name.
func canonicalForDeprecated(deprecated string) string {
	switch deprecated {
	case paramStartTimeDeprecated:
		return paramStartTime
	case paramEndTimeDeprecated:
		return paramEndTime
	case paramRawTracesDeprecated:
		return paramRawTraces
	case paramServiceNameDeprecated:
		return paramServiceName
	case paramOperationNameDeprecated:
		return paramOperationName
	case paramTimeMinDeprecated:
		return paramTimeMin
	case paramTimeMaxDeprecated:
		return paramTimeMax
	case paramNumTracesDeprecated, paramSearchDepthDeprecated:
		return paramSearchDepth
	case paramDurationMinDeprecated:
		return paramDurationMin
	case paramDurationMaxDeprecated:
		return paramDurationMax
	case paramQueryRawTracesDeprecated:
		return paramQueryRawTraces
	case paramSpanKindDeprecated:
		return paramSpanKind
	default:
		return ""
	}
}
