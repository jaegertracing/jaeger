// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding

// HeaderRole describes the semantic role of a forwarded header value.
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
	// Role describes the semantic meaning of the header value (e.g. username, email).
	// Jaeger does not act on this today; it is informational for downstream consumers.
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
