// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package ports

import (
	"strconv"
	"strings"
)

const (
	// AgentJaegerThriftCompactUDP is the default port for receiving Jaeger Thrift over UDP in compact encoding
	AgentJaegerThriftCompactUDP = 6831
	// AgentJaegerThriftBinaryUDP is the default port for receiving Jaeger Thrift over UDP in binary encoding
	AgentJaegerThriftBinaryUDP = 6832
	// AgentZipkinThriftCompactUDP is the default port for receiving Zipkin Thrift over UDP in binary encoding
	AgentZipkinThriftCompactUDP = 5775
	// AgentConfigServerHTTP is the default port for the agent's HTTP config server (e.g. /sampling endpoint)
	AgentConfigServerHTTP = 5778
	// AgentAdminHTTP is the default admin HTTP port (health check, metrics, etc.)
	AgentAdminHTTP = 14271

	// CollectorGRPC is the default port for gRPC server for sending spans
	CollectorGRPC = 14250
	// CollectorHTTP is the default port for HTTP server for sending spans (e.g. /api/traces endpoint)
	CollectorHTTP = 14268
	// CollectorAdminHTTP is the default admin HTTP port (health check, metrics, etc.)
	CollectorAdminHTTP = 14269
	// CollectorZipkin is the port for Zipkin server for sending spans
	CollectorZipkin = 9411

	// QueryGRPC is the default port of GRPC requests for Query trace retrieval
	QueryGRPC = 16685
	// QueryHTTP is the default port for UI and Query API (e.g. /api/* endpoints)
	QueryHTTP = 16686
	// QueryAdminHTTP is the default admin HTTP port (health check, metrics, etc.)
	QueryAdminHTTP = 16687

	// IngesterAdminHTTP is the default admin HTTP port (health check, metrics, etc.)
	IngesterAdminHTTP = 14270

	// RemoteStorageGRPC is the default port of GRPC requests for Remote Storage
	RemoteStorageGRPC = 17271
	// RemoteStorageHTTP is the default admin HTTP port (health check, metrics, etc.)
	RemoteStorageAdminHTTP = 17270
)

// PortToHostPort converts the port into a host:port address string
func PortToHostPort(port int) string {
	return ":" + strconv.Itoa(port)
}

// FormatHostPort returns hostPort in a usable format (host:port) if it wasn't already
func FormatHostPort(hostPort string) string {
	if hostPort == "" {
		return ""
	}

	if strings.Contains(hostPort, ":") {
		return hostPort
	}

	return ":" + hostPort
}
