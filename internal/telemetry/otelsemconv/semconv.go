// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelsemconv

import (
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)


const (
	SchemaURL = semconv.SchemaURL

	TelemetrySDKLanguageKey   = semconv.TelemetrySDKLanguageKey
	TelemetrySDKNameKey       = semconv.TelemetrySDKNameKey
	TelemetrySDKVersionKey    = semconv.TelemetrySDKVersionKey
	TelemetryDistroNameKey    = semconv.TelemetryDistroNameKey
	TelemetryDistroVersionKey = semconv.TelemetryDistroVersionKey

	ServiceNameKey            = semconv.ServiceNameKey
	DBQueryTextKey            = semconv.DBQueryTextKey
	DBSystemKey               = semconv.DBSystemNameKey
	PeerServiceKey            = semconv.PeerServiceKey
	HTTPResponseStatusCodeKey = semconv.HTTPResponseStatusCodeKey

	HostIDKey   = semconv.HostIDKey
	HostIPKey   = semconv.HostIPKey
	HostNameKey = semconv.HostNameKey
)

var (
	HTTPResponseStatusCode = semconv.HTTPResponseStatusCode
)
