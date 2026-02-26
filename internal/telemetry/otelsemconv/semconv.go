// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package otelsemconv

import (
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

// We do not use a lot of semconv constants, and its annoying to keep
// the semver of the imports the same. This package serves as a
// one stop shop replacement / alias.
const (
	SchemaURL = semconv.SchemaURL

	// Telemetry SDK
	TelemetrySDKLanguageKey   = string(semconv.TelemetrySDKLanguageKey)
	TelemetrySDKNameKey       = string(semconv.TelemetrySDKNameKey)
	TelemetrySDKVersionKey    = string(semconv.TelemetrySDKVersionKey)
	TelemetryDistroNameKey    = string(semconv.TelemetryDistroNameKey)
	TelemetryDistroVersionKey = string(semconv.TelemetryDistroVersionKey)

	// Service
	ServiceNameKey = string(semconv.ServiceNameKey)

	// Database
	DBQueryTextKey = string(semconv.DBQueryTextKey)
	DBSystemKey    = "db.system"

	// Network
	PeerServiceKey = string(semconv.ServicePeerNameKey)

	// HTTP
	HTTPResponseStatusCodeKey = string(semconv.HTTPResponseStatusCodeKey)

	// Host
	HostIDKey   = string(semconv.HostIDKey)
	HostIPKey   = string(semconv.HostIPKey)
	HostNameKey = string(semconv.HostNameKey)

	// Status
	OtelStatusCode        = "otel.status_code"
	OtelStatusDescription = "otel.status_description"

	// OpenTracing
	AttributeOpentracingRefType            = "opentracing.ref_type"
	AttributeOpentracingRefTypeChildOf     = "child_of"
	AttributeOpentracingRefTypeFollowsFrom = "follows_from"

	// OTel Scope
	AttributeOtelScopeName    = "otel.scope.name"
	AttributeOtelScopeVersion = "otel.scope.version"
)

// Helper functions for creating typed attributes for the OpenTelemetry SDK.
// ServiceName creates a key-value pair for the service name attribute.
func ServiceNameAttribute(value string) attribute.KeyValue {
	return semconv.ServiceNameKey.String(value)
}

// PeerService creates a key-value pair for the peer service attribute.
func PeerServiceAttribute(value string) attribute.KeyValue {
	return semconv.ServicePeerNameKey.String(value)
}

// DBSystem creates a key-value pair for the DB system attribute.
func DBSystemAttribute(value string) attribute.KeyValue {
	return semconv.DBSystemNameKey.String(value)
}

// HTTPStatusCode creates a key-value pair for the HTTP status code attribute.
func HTTPStatusCodeAttribute(value int) attribute.KeyValue {
	return semconv.HTTPResponseStatusCodeKey.Int(value)
}

// This var provides the original semconv function variable for creating an int attribute.
var HTTPResponseStatusCode = semconv.HTTPResponseStatusCode
