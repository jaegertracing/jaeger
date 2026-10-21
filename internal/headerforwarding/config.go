// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding

// HeaderRole labels the semantic meaning of a forwarded header value (e.g. a
// username or an email identity). It is a runtime annotation that in-process
// consumers can read off the captured header — see ForwardedHeader.Role.
type HeaderRole string

const (
	// RoleUsername indicates the header carries a username identity.
	RoleUsername HeaderRole = "username"
	// RoleEmail indicates the header carries an email identity.
	RoleEmail HeaderRole = "email"
)

// ForwardedHeader describes one header to be forwarded from inbound requests to outbound gRPC storage calls.
type ForwardedHeader struct {
	// HTTPName is the name of the HTTP request header to extract on inbound HTTP requests.
	HTTPName string `mapstructure:"http_name"`
	// GRPCName is the name of the gRPC metadata key to extract on inbound gRPC requests.
	// When empty, HTTPName is used as the fallback.
	GRPCName string `mapstructure:"grpc_name"`
	// Role labels the semantic meaning of the header value (e.g. username, email).
	// It travels with the captured header on the request context (see
	// CapturedFromContext), so in-process consumers — storage plugins, extensions,
	// exporters — can read it, even though Jaeger's own code does not. It is not sent
	// to the backend; only the header value is forwarded.
	Role HeaderRole `mapstructure:"header_role"`
	// GRPCOutboundName is the metadata key used when forwarding the value to the gRPC storage backend.
	// When empty, GRPCName/HTTPName is used as the fallback (in that order).
	GRPCOutboundName string `mapstructure:"grpc_outbound_name"`
	// HTTPOutboundName is the header name used when forwarding the value to an HTTP storage backend.
	// When empty, HTTPName is used as the fallback.
	HTTPOutboundName string `mapstructure:"http_outbound_name"`
}

// inboundGRPCName returns the key to look for in incoming gRPC metadata.
func (h *ForwardedHeader) inboundGRPCName() string {
	if h.GRPCName != "" {
		return h.GRPCName
	}
	return h.HTTPName
}

// outboundGRPCName returns the metadata key to use when forwarding to storage.
func (h *ForwardedHeader) outboundGRPCName() string {
	if h.GRPCOutboundName != "" {
		return h.GRPCOutboundName
	}
	return h.inboundGRPCName()
}

// outboundHTTPName returns the header name to use when forwarding to an HTTP storage backend.
func (h *ForwardedHeader) outboundHTTPName() string {
	if h.HTTPOutboundName != "" {
		return h.HTTPOutboundName
	}
	return h.HTTPName
}
