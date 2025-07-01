// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelsemconv

import (
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// We do not use a lot of semconv constants, and its annoying to keep
// the semver of the imports the same. This package serves as a
// one stop shop replacement / alias.
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

var HTTPResponseStatusCode = semconv.HTTPResponseStatusCode
